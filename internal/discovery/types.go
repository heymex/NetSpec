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

type Interface struct {
	Index             int    `json:"index"`
	Name              string `json:"name"`
	Alias             string `json:"alias"`
	Type              int    `json:"type"`
	TypeLabel         string `json:"type_label"`
	AdminStatus       string `json:"admin_status"`
	OperStatus        string `json:"oper_status"`
	AlreadyConfigured bool   `json:"already_configured"`
	FilteredDefault   bool   `json:"filtered_default"`
}

type WalkResult struct {
	Address       string      `json:"address"`
	Interfaces    []Interface `json:"interfaces"`
	FilteredCount int         `json:"filtered_count"`
}

type CommitInterface struct {
	Name          string `json:"name"`
	Alias         string `json:"alias"`
	Monitor       bool   `json:"monitor"`
	DesiredState  string `json:"desired_state"`
	AdminState    string `json:"admin_state"`
	AlertSeverity string `json:"alert_severity"`
}

type CommitRequest struct {
	Address           string            `json:"address"`
	Community         string            `json:"community"`
	DeviceKey         string            `json:"device_key"`
	DeviceDescription string            `json:"device_description"`
	ExistingDeviceKey string            `json:"existing_device_key"`
	Action            string            `json:"action"`
	Interfaces        []CommitInterface `json:"interfaces"`
}

type CommitResult struct {
	Success             bool   `json:"success"`
	Action              string `json:"action"`
	DeviceKey           string `json:"device_key"`
	InterfacesWritten   int    `json:"interfaces_written"`
	InterfacesMonitored int    `json:"interfaces_monitored"`
	Message             string `json:"message"`
}
