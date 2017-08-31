package homie

import "strings"

type Node interface {
	Name() string
	Type() string
	Properties() []string
	AddProperty(name string, settable bool, unit string, datatype string, format string)
	Set(property string, value string)
	Publish()
}

type node struct {
	name       string
	nodeType   string
	properties map[string]Property
	publish    publishFunc
}

func NewNode(name string, nodeType string, publish publishFunc) Node {
	newnode := &node{
		name:       name,
		nodeType:   nodeType,
		publish:    publish,
		properties: map[string]Property{},
	}
	return newnode
}

func (node *node) Name() string {
	return node.name
}

func (node *node) Properties() []string {
	properties := make([]string, len(node.properties))
	idx := 0
	for property := range node.properties {
		properties[idx] = property
		idx += 1
	}
	return properties
}

func (node *node) Set(property string, value string) {
	node.properties[property].Set(value)
}
func (node *node) Type() string {
	return node.nodeType
}
func (node *node) AddProperty(name string, settable bool, unit string, datatype string, format string) {
	property := NewProperty(name, unit, datatype, format, func(name, value string) {
		node.publish(node.Name()+"/"+name, value)
	})
	node.properties[property.Name()] = property
}

func (node *node) Publish() {
	propertiesList := make([]string, len(node.properties))
	i := 0
	for _, prop := range node.properties {
		prop.Publish()
		propertiesList[i] = prop.Name()
		i += 1
	}
	node.publish("$type", node.Type())
	node.publish("$properties", strings.Join(propertiesList, ","))
}
