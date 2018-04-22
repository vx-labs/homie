package homie

type DeviceState string

func (d *DeviceState) toString() string {
	return string(*d)
}

const (
	InitState         DeviceState = "init"
	ReadyState        DeviceState = "ready"
	DisconnectedState DeviceState = "disconnected"
	SleepingState     DeviceState = "sleeping"
	LostState         DeviceState = "lost"
	AlertState        DeviceState = "alert"
)

func (homieClient *client) publishDeviceState(state DeviceState) {
	homieClient.publish("$state", state.toString())
}
