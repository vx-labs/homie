package homie

type Node interface {
	Name() string
	Type() string
	Properties() []string
	Settables() []SettableProperty
	Set(property string, value string)
}

type node struct {
	name       string
	nodeType   string
	properties map[string]string
	settables  []SettableProperty
	callback   func(property string, value string)
}

func NewNode(name string, nodeType string, properties []string, settables []SettableProperty, callback func(property string, value string)) Node {
	newnode := &node{
		name:       name,
		nodeType:   nodeType,
		callback:   callback,
		settables:  settables,
		properties: map[string]string{},
	}
	for _, property := range properties {
		newnode.properties[property] = ""
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
	node.properties[property] = value
	node.callback(property, value)
}
func (node *node) Settables() []SettableProperty {
	return node.settables
}
func (node *node) Type() string {
	return node.nodeType
}
