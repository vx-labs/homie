package homie

import (
	"github.com/sirupsen/logrus"
	"strings"
)

type Node interface {
	Name() string
	Type() string
	Properties() []string
	AddProperty(name string, unit string, datatype string, format string)
	AddSettable(name string, unit string, datatype string, format string, callback func(property Property, payload string))
	Set(property string, value string)
	Publish()
}

type node struct {
	name       string
	nodeType   string
	properties map[string]Property
	logger     *logrus.Entry
	publish    publishFunc
	subscribe  subscribeFunc
}

func NewNode(name string, nodeType string, logger *logrus.Entry, publish publishFunc, subscribe subscribeFunc) Node {
	newnode := &node{
		name:       name,
		nodeType:   nodeType,
		publish:    publish,
		subscribe:  subscribe,
		logger:     logger,
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
func (node *node) AddProperty(name string, unit string, datatype string, format string) {
	property := NewProperty(name, false, unit, datatype, format, func(name, value string) {
		node.publish(node.Name()+"/"+name, value)
	})
	node.properties[property.Name()] = property
}

func (node *node) Publish() {
	node.logger.Debugf("publishing node %s", node.name)
	propertiesList := make([]string, len(node.properties))
	i := 0
	for _, prop := range node.properties {
		property := prop
		property.Publish()
		if property.Settable() {
			node.subscribe(node.name+"/"+property.Name()+"/set", func(topic, payload string) {
				node.logger.Debugf("routed %s to property %s", topic, property.Name())
				property.Callback(payload)
			})
		}
		propertiesList[i] = property.Name()
		i += 1
	}
	node.publish(node.name+"/$type", node.Type())
	node.publish(node.name+"/$name", node.Name())
	node.publish(node.name+"/$properties", strings.Join(propertiesList, ","))
}

func (node *node) AddSettable(name string, unit string, datatype string, format string, callback func(property Property, payload string)) {
	property := NewProperty(name, true, unit, datatype, format, func(name, value string) {
		node.publish(node.Name()+"/"+name, value)
	})
	property.SetCallback(callback)
	node.properties[property.Name()] = property
}
