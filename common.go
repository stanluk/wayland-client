package wayland

type ProxyId uint32

type Proxy interface {
	GetDisplay() *Display
	SetDisplay(*Display)
	SetId(id ProxyId)
	GetId() ProxyId
	DispatchEvent(m *Message) error
}

type BaseProxy struct {
	id      ProxyId
	display *Display
}

func (p *BaseProxy) GetId() ProxyId {
	return p.id
}

func (p *BaseProxy) SetId(id ProxyId) {
	p.id = id
}

func (p *BaseProxy) GetDisplay() *Display {
	return p.display
}

func (p *BaseProxy) SetDisplay(d *Display) {
	p.display = d
}

func (p *BaseProxy) DispatchEvent(m *Message) error {
	return nil
}
