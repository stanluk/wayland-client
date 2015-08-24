package wayland

import (
	"bytes"
	"encoding/binary"
	"errors"
	"net"
	"os"
	"strings"
	"syscall"
)

type Message struct {
	Id      ProxyId
	Opcode  uint32
	size    uint32
	data    *bytes.Buffer
	control *bytes.Buffer
}

func ReadWaylandMessage(conn *net.UnixConn) (*Message, error) {
	var buf [8]byte
	msg := Message{}

	n, err := conn.Read(buf[:])
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, errors.New("Zero bytes read.")
	}
	msg.Id = ProxyId(binary.LittleEndian.Uint32(buf[0:4]))
	msg.Opcode = uint32(binary.LittleEndian.Uint16(buf[4:6]))
	msg.size = uint32(binary.LittleEndian.Uint16(buf[6:8]))

	// subtract 8 bytes from header
	data := make([]byte, msg.size-8)
	control := make([]byte, 0)

	n, _, _, _, _ = conn.ReadMsgUnix(data, control)
	if n != int(msg.size)-8 {
		return nil, errors.New("Invalid message size.")
	}
	msg.data = bytes.NewBuffer(data)
	msg.control = bytes.NewBuffer(control)

	return &msg, nil
}

func (m *Message) Write(arg interface{}) error {
	switch t := arg.(type) {
	case Proxy:
		return binary.Write(m.data, binary.LittleEndian, t.GetId())
	case uint32, float32:
		return binary.Write(m.data, binary.LittleEndian, arg)
	case int:
		return binary.Write(m.data, binary.LittleEndian, uint32(t))
	case string:
		str, _ := arg.(string)
		tail := 4 - (len(str)&0x3)&0x3
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
	case *os.File:
		rights := syscall.UnixRights(int(t.Fd()))
		return binary.Write(m.control, binary.LittleEndian, rights)
	default:
		panic("Invalid Wayland request parameter type.")
	}
	return nil
}

func (m *Message) GetProxy(d *Display) (Proxy, error) {
	buf := m.data.Next(4)
	if len(buf) != 4 {
		return nil, errors.New("Unable to read object id")
	}
	id := ProxyId(binary.LittleEndian.Uint32(buf))
	return d.context.objects[id], nil
}

func (m *Message) GetString() (string, error) {
	buf := m.data.Next(4)
	if len(buf) != 4 {
		return "", errors.New("Unable to read string length")
	}
	l := int(binary.LittleEndian.Uint32(buf))
	buf = m.data.Next(l)
	if len(buf) != l {
		return "", errors.New("Unable to read string")
	}
	return strings.TrimRight(string(buf), "\x00"), nil
}

func (m *Message) GetInt32() (int32, error) {
	buf := m.data.Next(4)
	if len(buf) != 4 {
		return 0, errors.New("Unable to read int")
	}
	return int32(binary.LittleEndian.Uint32(buf)), nil
}

func (m *Message) GetUint32() (uint32, error) {
	buf := m.data.Next(4)
	if len(buf) != 4 {
		return 0, errors.New("Unable to read unsigned int")
	}
	return binary.LittleEndian.Uint32(buf), nil
}

func (m *Message) GetFloat32() (float32, error) {
	buf := m.data.Next(4)
	if len(buf) != 4 {
		return 0, errors.New("Unable to read fixed")
	}
	return float32(fixedToFloat64(int32(binary.LittleEndian.Uint32(buf)))), nil
}

func (m *Message) GetArray() ([]uint32, error) {
	buf := m.data.Next(4)
	if len(buf) != 4 {
		return nil, errors.New("Unable to array len")
	}
	l := binary.LittleEndian.Uint32(buf)
	arr := make([]uint32, l/4)
	for _, i := range arr {
		buf = m.data.Next(4)
		if len(buf) != 4 {
			return nil, errors.New("Unable to array element")
		}
		arr[i] = binary.LittleEndian.Uint32(buf)
	}
	return arr, nil
}

func NewWaylandRequest(p Proxy, opcode uint32) *Message {
	msg := Message{}
	msg.Opcode = opcode
	msg.Id = p.GetId()
	msg.data = bytes.NewBuffer(nil)
	msg.control = bytes.NewBuffer(nil)

	return &msg
}

func SendWaylandMessage(conn *net.UnixConn, m *Message) error {
	header := &bytes.Buffer{}
	// calculate message total size
	m.size = uint32(m.data.Len() + 8)
	binary.Write(header, binary.LittleEndian, m.Id)
	binary.Write(header, binary.LittleEndian, m.size<<16|m.Opcode&0x0000ffff)

	d, c, err := conn.WriteMsgUnix(append(header.Bytes(), m.data.Bytes()...), m.control.Bytes(), nil)
	if c != m.control.Len() {
		panic("Unable to write control message.")
	}
	if d != (header.Len() + m.data.Len()) {
		panic("Unable to write message data.")
	}
	return err
}
