package registry

type Address struct {
	IP   string
	Port uint64
}

// Registry Extension - Registry
type Registry interface {
	//注册服务
	Register(addr *Address) error
	//取消注册
	UnRegister(addr *Address) error
	//查询服务地址
	Lookup() ([]string, error)
}
