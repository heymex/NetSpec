package discovery

type ProbeResult struct {
	Address           string `json:"address"`
	SysName           string `json:"sys_name"`
	SysDescr          string `json:"sys_descr"`
	SysObjectID       string `json:"sys_object_id"`
	SysLocation       string `json:"sys_location"`
	VendorHint        string `json:"vendor_hint"`
	AlreadyConfigured bool   `json:"already_configured"`
	ExistingDeviceKey string `json:"existing_device_key,omitempty"`
}

// InterfaceConfigWish is current desired-state for an interface when re-walking SNMP.
type InterfaceConfigWish struct {
	Monitor       bool     `json:"monitor"`
	Description   string   `json:"description,omitempty"`
	DesiredState  string   `json:"desired_state,omitempty"`
	AdminState    string   `json:"admin_state,omitempty"`
	AlertSeverity string   `json:"alert_severity,omitempty"`
	IsPortChannel bool     `json:"is_port_channel,omitempty"`
	Members       []string `json:"members,omitempty"`
}

type Interface struct {
	Index             int                  `json:"index"`
	Name              string               `json:"name"`
	Alias             string               `json:"alias"`
	Type              int                  `json:"type"`
	TypeLabel         string               `json:"type_label"`
	IsPortChannel     bool                 `json:"is_port_channel"`
	ChannelMembers    []string             `json:"channel_members,omitempty"`
	AdminStatus       string               `json:"admin_status"`
	OperStatus        string               `json:"oper_status"`
	AlreadyConfigured bool                 `json:"already_configured"`
	FilteredDefault   bool                 `json:"filtered_default"`
	ExistingConfig    *InterfaceConfigWish `json:"existing_config,omitempty"`
}

type WalkResult struct {
	Address       string      `json:"address"`
	Interfaces    []Interface `json:"interfaces"`
	FilteredCount int         `json:"filtered_count"`
}

type CommitInterface struct {
	Name          string   `json:"name"`
	Alias         string   `json:"alias"`
	Monitor       bool     `json:"monitor"`
	DesiredState  string   `json:"desired_state"`
	AdminState    string   `json:"admin_state"`
	AlertSeverity string   `json:"alert_severity"`
	IsPortChannel bool     `json:"is_port_channel,omitempty"`
	Members       []string `json:"members,omitempty"`
}

type CommitRequest struct {
	Address           string            `json:"address"`
	Community         string            `json:"community"`
	DeviceKey         string            `json:"device_key"`
	DeviceDescription string            `json:"device_description"`
	ExistingDeviceKey string            `json:"existing_device_key"`
	Action            string            `json:"action"`
	Interfaces        []CommitInterface `json:"interfaces"`
	// SyncDiscoveredInterfaces (patch only): Interfaces is the full SNMP walk set; monitored
	// interfaces are upserted and unchecked names are removed from desired state.
	SyncDiscoveredInterfaces bool `json:"sync_discovered_interfaces,omitempty"`
}

type CommitResult struct {
	Success             bool   `json:"success"`
	Action              string `json:"action"`
	DeviceKey           string `json:"device_key"`
	InterfacesWritten   int    `json:"interfaces_written"`
	InterfacesMonitored int    `json:"interfaces_monitored"`
	Message             string `json:"message"`
}
