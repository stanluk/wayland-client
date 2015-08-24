package wayland

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"strings"
	"syscall"
)

type Message struct {
	Id           ProxyId
	Opcode       uint32
	size         uint32
	data         *bytes.Buffer
	control      *bytes.Buffer
	control_msgs []syscall.SocketControlMessage
}

func ReadWaylandMessage(conn *net.UnixConn) (*Message, error) {
	var buf [8]byte
	msg := Message{}
	control := make([]byte, 24)

	n, oobn, _, _, err := conn.ReadMsgUnix(buf[:], control)
	if err != nil {
		return nil, err
	}
	if n != 8 {
		return nil, errors.New("Unable to read message header.")
	}
	if oobn > 0 {
		if oobn > len(control) {
			panic("Unsufficient control msg buffer")
		}
		msg.control_msgs, err = syscall.ParseSocketControlMessage(control)
		if err != nil {
			panic(fmt.Sprintf("Control message parse error: %s", err.Error()))
		}
	}

	msg.Id = ProxyId(binary.LittleEndian.Uint32(buf[0:4]))
	msg.Opcode = uint32(binary.LittleEndian.Uint16(buf[4:6]))
	msg.size = uint32(binary.LittleEndian.Uint16(buf[6:8]))

	// subtract 8 bytes from header
	data := make([]byte, msg.size-8)

	n, err = conn.Read(data)
	if err != nil {
		return nil, err
	}
	if n != int(msg.size)-8 {
		return nil, errors.New("Invalid message size.")
	}
	msg.data = bytes.NewBuffer(data)

	return &msg, nil
}

func (m *Message) Write(arg interface{}) error {
	switch t := arg.(type) {
	case Proxy:
		return binary.Write(m.data, binary.LittleEndian, uint32(t.Id()))
	case uint32, int32:
		return binary.Write(m.data, binary.LittleEndian, t)
	case float32:
		f := float64ToFixed(float64(t))
		return binary.Write(m.data, binary.LittleEndian, f)
	case string:
		str, _ := arg.(string)
		tail := 4 - (len(str) & 0x3)
		err := binary.Write(m.data, binary.LittleEndian, uint32(len(str)+tail))
		if err != nil {
			return err
		}
		err = binary.Write(m.data, binary.LittleEndian, []byte(str))
		if err != nil {
			return err
		}
		padding := make([]byte, tail)
		return binary.Write(m.data, binary.LittleEndian, padding)
	case uintptr:
		rights := syscall.UnixRights(int(t))
		return binary.Write(m.control, binary.LittleEndian, rights)
	default:
		panic("Invalid Wayland request parameter type.")
	}
	return nil
}

func (m *Message) GetProxy(c *Connection) Proxy {
	buf := m.data.Next(4)
	if len(buf) != 4 {
		panic("Unable to read object id")
	}
	return c.objects[ProxyId(binary.LittleEndian.Uint32(buf))]
}

func (m *Message) GetFD() uintptr {
	if m.control_msgs == nil {
		return 0
	}
	fds, err := syscall.ParseUnixRights(&m.control_msgs[0])
	if err != nil {
		panic("Unable to parse unix rights")
	}
	m.control_msgs = append(m.control_msgs[0:], m.control_msgs[1:]...)
	if len(fds) != 1 {
		panic("Expected 1 file descriptor, got more")
	}
	return uintptr(fds[0])
}

func (m *Message) GetString() string {
	buf := m.data.Next(4)
	if len(buf) != 4 {
		panic("Unable to read string length")
	}
	l := int(binary.LittleEndian.Uint32(buf))
	buf = m.data.Next(l)
	if len(buf) != l {
		panic("Unable to read string")
	}
	ret := strings.TrimRight(string(buf), "\x00")
	//padding to 32 bit boundary
	if (l & 0x3) != 0 {
		buf = m.data.Next(4 - (l & 0x3))
	}
	return ret
}

func (m *Message) GetInt32() int32 {
	buf := m.data.Next(4)
	if len(buf) != 4 {
		panic("Unable to read int")
	}
	return int32(binary.LittleEndian.Uint32(buf))
}

func (m *Message) GetUint32() uint32 {
	buf := m.data.Next(4)
	if len(buf) != 4 {
		panic("Unable to read unsigned int")
	}
	return binary.LittleEndian.Uint32(buf)
}

func (m *Message) GetFloat32() float32 {
	buf := m.data.Next(4)
	if len(buf) != 4 {
		panic("Unable to read fixed")
	}
	return float32(fixedToFloat64(int32(binary.LittleEndian.Uint32(buf))))
}

func (m *Message) GetArray() []int32 {
	buf := m.data.Next(4)
	if len(buf) != 4 {
		panic("Unable to array len")
	}
	l := binary.LittleEndian.Uint32(buf)
	arr := make([]int32, l/4)
	for _, i := range arr {
		buf = m.data.Next(4)
		if len(buf) != 4 {
			panic("Unable to array element")
		}
		arr[i] = int32(binary.LittleEndian.Uint32(buf))
	}
	return arr
}

func NewRequest(p Proxy, opcode uint32) *Message {
	msg := Message{}
	msg.Opcode = opcode
	msg.Id = p.Id()
	msg.data = &bytes.Buffer{}
	msg.control = &bytes.Buffer{}

	return &msg
}

func SendWaylandMessage(conn *net.UnixConn, m *Message) error {
	header := &bytes.Buffer{}
	// calculate message total size
	m.size = uint32(m.data.Len() + 8)
	binary.Write(header, binary.LittleEndian, m.Id)
	binary.Write(header, binary.LittleEndian, m.size<<16|m.Opcode&0x0000ffff)

	d, c, err := conn.WriteMsgUnix(append(header.Bytes(), m.data.Bytes()...), m.control.Bytes(), nil)
	if err != nil {
		panic(err.Error())
	}
	if c != m.control.Len() || d != (header.Len()+m.data.Len()) {
		panic("WriteMsgUnix failed.")
	}
	return err
}
