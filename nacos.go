package nacos

import (
	"errors"
	"fmt"
	"github.com/asim/go-micro/v3/registry"
	"github.com/nacos-group/nacos-sdk-go/clients"
	"github.com/nacos-group/nacos-sdk-go/clients/naming_client"
	"github.com/nacos-group/nacos-sdk-go/common/constant"
	"github.com/nacos-group/nacos-sdk-go/vo"
	"net"
	"strconv"
	"time"
)

type nacosRegistry struct {
	client     naming_client.INamingClient
	opts       registry.Options
	namespace  string
	cacheDir   string
	logDir     string
	RotateTime string //the rotate time for log, eg: 30m, 1h, 24h, default is 24h
	MaxAge     int64  //the max age of a log file, default value is 3
	LogLevel   string //the level of log, it's must be debug,info,warn,error, default value is info
}

// NewRegistry NewRegistry
func NewRegistry(opts ...registry.Option) registry.Registry {
	n := &nacosRegistry{
		opts: registry.Options{},
	}
	configure(n, opts...)
	return n
}

func configure(n *nacosRegistry, opts ...registry.Option) error {
	// set opts
	for _, o := range opts {
		o(&n.opts)
	}

	if n.opts.Context != nil {
		if namespace, ok := n.opts.Context.Value(&NacosNamespaceContextKey{}).(string); ok {
			n.namespace = namespace
		}
		if cacheDir, ok := n.opts.Context.Value(&NacosCacheDirContextKey{}).(string); ok {
			n.cacheDir = cacheDir
		}
		if logDir, ok := n.opts.Context.Value(&NacosLogDirContextKey{}).(string); ok {
			n.logDir = logDir
		}
	}

	clientConfig := constant.ClientConfig{
		CacheDir: n.cacheDir,
		LogDir:   n.logDir,
	}
	serverConfigs := make([]constant.ServerConfig, 0)
	contextPath := "/nacos"

	// iterate the options addresses
	for _, address := range n.opts.Addrs {
		// check we have a port
		addr, port, err := net.SplitHostPort(address)
		if ae, ok := err.(*net.AddrError); ok && ae.Err == "missing port in address" {
			serverConfigs = append(serverConfigs, constant.ServerConfig{
				IpAddr:      addr,
				Port:        8848,
				ContextPath: contextPath,
			})
		} else if err == nil {
			p, err := strconv.ParseUint(port, 10, 64)
			if err != nil {
				continue
			}
			serverConfigs = append(serverConfigs, constant.ServerConfig{
				IpAddr:      addr,
				Port:        p,
				ContextPath: contextPath,
			})
		}
	}

	if n.opts.Timeout == 0 {
		n.opts.Timeout = time.Second * 1
	}

	clientConfig.NamespaceId = n.namespace
	clientConfig.TimeoutMs = uint64(n.opts.Timeout.Milliseconds())
	client, err := clients.CreateNamingClient(map[string]interface{}{
		"serverConfigs": serverConfigs,
		"clientConfig":  clientConfig,
	})
	if err != nil {
		return err
	}
	n.client = client

	return nil
}

func getNodeIPPort(s *registry.Service) (host string, port int, err error) {
	if len(s.Nodes) == 0 {
		return "", 0, errors.New("you must deregister at least one node")
	}
	node := s.Nodes[0]
	host, pt, err := net.SplitHostPort(node.Address)
	if err != nil {
		return "", 0, err
	}
	port, err = strconv.Atoi(pt)
	if err != nil {
		return "", 0, err
	}
	return
}

func (n *nacosRegistry) Init(opts ...registry.Option) error {
	configure(n, opts...)
	return nil
}

func (n *nacosRegistry) Options() registry.Options {
	return n.opts
}

func (n *nacosRegistry) Register(s *registry.Service, opts ...registry.RegisterOption) error {
	var options registry.RegisterOptions
	for _, o := range opts {
		o(&options)
	}
	withContext := false
	param := vo.RegisterInstanceParam{}
	if options.Context != nil {
		if p, ok := options.Context.Value("register_instance_param").(vo.RegisterInstanceParam); ok {
			param = p
			withContext = ok
		}
	}
	if !withContext {
		host, port, err := getNodeIPPort(s)
		if err != nil {
			return err
		}
		if s.Nodes[0].Metadata == nil {
			s.Nodes[0].Metadata = make(map[string]string)
		}
		s.Nodes[0].Metadata["version"] = s.Version
		param.Ip = host
		param.Port = uint64(port)
		param.Metadata = s.Nodes[0].Metadata
		param.ServiceName = s.Name
		param.Enable = true
		param.Healthy = true
		param.Weight = 1.0
		param.Ephemeral = true
	}
	_, err := n.client.RegisterInstance(param)
	return err
}

func (n *nacosRegistry) Deregister(s *registry.Service, opts ...registry.DeregisterOption) error {
	var options registry.DeregisterOptions
	for _, o := range opts {
		o(&options)
	}
	withContext := false
	param := vo.DeregisterInstanceParam{}
	if options.Context != nil {
		if p, ok := options.Context.Value("deregister_instance_param").(vo.DeregisterInstanceParam); ok {
			param = p
			withContext = ok
		}
	}
	if !withContext {
		host, port, err := getNodeIPPort(s)
		if err != nil {
			return err
		}
		param.Ip = host
		param.Port = uint64(port)
		param.ServiceName = s.Name
	}

	_, err := n.client.DeregisterInstance(param)
	return err
}

func (n *nacosRegistry) GetService(name string, opts ...registry.GetOption) ([]*registry.Service, error) {
	var options registry.GetOptions
	for _, o := range opts {
		o(&options)
	}
	withContext := false
	param := vo.GetServiceParam{}
	if options.Context != nil {
		if p, ok := options.Context.Value("select_instances_param").(vo.GetServiceParam); ok {
			param = p
			withContext = ok
		}
	}
	if !withContext {
		param.ServiceName = name
	}
	service, err := n.client.GetService(param)
	if err != nil {
		return nil, err
	}
	services := make([]*registry.Service, 0)
	for _, v := range service.Hosts {
		nodes := make([]*registry.Node, 0)
		nodes = append(nodes, &registry.Node{
			Id:       v.InstanceId,
			Address:  net.JoinHostPort(v.Ip, fmt.Sprintf("%d", v.Port)),
			Metadata: v.Metadata,
		})
		s := registry.Service{
			Name:     v.ServiceName,
			Version:  v.Metadata["version"],
			Metadata: v.Metadata,
			Nodes:    nodes,
		}
		services = append(services, &s)
	}

	return services, nil
}

func (n *nacosRegistry) ListServices(opts ...registry.ListOption) ([]*registry.Service, error) {
	var options registry.ListOptions
	for _, o := range opts {
		o(&options)
	}
	withContext := false
	param := vo.GetAllServiceInfoParam{}
	if options.Context != nil {
		if p, ok := options.Context.Value("get_all_service_info_param").(vo.GetAllServiceInfoParam); ok {
			param = p
			withContext = ok
		}
	}
	if !withContext {
		services, err := n.client.GetAllServicesInfo(param)
		if err != nil {
			return nil, err
		}
		param.PageNo = 1
		param.PageSize = uint32(services.Count)
	}
	services, err := n.client.GetAllServicesInfo(param)
	if err != nil {
		return nil, err
	}
	var registryServices []*registry.Service
	for _, v := range services.Doms {
		registryServices = append(registryServices, &registry.Service{Name: v})
	}
	return registryServices, nil
}

func (n *nacosRegistry) Watch(opts ...registry.WatchOption) (registry.Watcher, error) {
	return NewNacosWatcher(n, opts...)
}

func (n *nacosRegistry) String() string {
	return "nacos"
}
