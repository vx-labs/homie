package homie

type Property interface {
	Name() string
	Value() string
	Set(value string)
	Settable() bool
	Unit() string
	Datatype() string
	Format() string
	Publish() error
}
type property struct {
	name     string
	value    string
	settable bool
	unit     string
	datatype string
	format   string
	publish  publishFunc
}

func (p *property) Name() string {
	return p.name
}
func (p *property) Value() string {
	return p.value
}
func (p *property) Set(value string) {
	p.value = value
	p.publish(p.name, p.value)
}
func (p *property) Settable() bool {
	return p.settable
}
func (p *property) Unit() string {
	return p.unit
}
func (p *property) Datatype() string {
	return p.datatype
}
func (p *property) Format() string {
	return p.format
}
func (p *property) Publish() error {
	p.publish(p.name, p.value)
	if p.settable {
		p.publish(p.name+"/$settable", "true")
	} else {
		p.publish(p.name+"/$settable", "false")
	}
	p.publish(p.name+"/$unit", p.unit)
	p.publish(p.name+"/$datatype", p.datatype)
	p.publish(p.name+"/$name", p.name)
	p.publish(p.name+"/$format", p.format)
	return nil
}

func NewProperty(name string, settable bool, unit string, datatype string, format string, cb publishFunc) Property {
	return &property{
		name:     name,
		settable: settable,
		unit:     unit,
		datatype: datatype,
		format:   format,
		publish:  cb,
	}
}
