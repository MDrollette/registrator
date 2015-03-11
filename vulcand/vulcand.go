package etcd

import (
	"errors"
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"strconv"

	"github.com/coreos/go-etcd/etcd"
	"github.com/gliderlabs/registrator/bridge"
)

func init() {
	bridge.Register(new(Factory), "vulcand")
}

var (
	ignoredError = errors.New("ignored container")
)

type Factory struct{}

func (f *Factory) New(uri *url.URL) bridge.RegistryAdapter {
	urls := make([]string, 0)
	if uri.Host != "" {
		urls = append(urls, "http://"+uri.Host)
	}
	return &VulcandAdapter{client: etcd.NewClient(urls), path: uri.Path, root: os.Getenv("SERVICE_DOMAIN")}
}

type VulcandAdapter struct {
	client *etcd.Client
	path   string
	root   string
}

func (r *VulcandAdapter) Ping() error {
	rr := etcd.NewRawRequest("GET", "version", nil, nil)
	_, err := r.client.SendRequest(rr)
	if err != nil {
		return err
	}
	return nil
}

func (r *VulcandAdapter) Register(service *bridge.Service) error {
	// ignore services with no deploy attr
	if _, ok := service.Attrs["deploy"]; !ok {
		return ignoredError
	}

	r.initService(service)

	path := r.path + "/backends/" + service.Name + "/servers/" + service.ID
	port := strconv.Itoa(service.Port)
	addr := net.JoinHostPort(service.IP, port)
	_, err := r.client.Set(path, fmt.Sprintf("{\"URL\": \"http://%s\"}", addr), uint64(service.TTL))
	if err != nil {
		log.Println("vulcand: failed to register service:", err)
	}
	return err
}

func (r *VulcandAdapter) Deregister(service *bridge.Service) error {
	path := r.path + "/backends/" + service.Name + "/servers/" + service.ID
	_, err := r.client.Delete(path, false)
	if err != nil {
		log.Println("vulcand: failed to deregister service:", err)
	}
	return err
}

func (r *VulcandAdapter) Refresh(service *bridge.Service) error {
	return r.Register(service)
}

func (r *VulcandAdapter) initService(service *bridge.Service) error {
	bpath := r.path + "/backends/" + service.Name + "/backend"
	if _, err := r.client.Set(bpath, "{\"Type\": \"http\"}", uint64(0)); nil != err {
		return err
	}

	fpath := r.path + "/frontends/" + service.Name + "/frontend"
	if _, err := r.client.Set(fpath, fmt.Sprintf("{\"Type\": \"http\", \"BackendId\": \"%s\", \"Route\": \"Host(`%s.%s`)\"}", service.Name, service.Name, r.root), uint64(0)); nil != err {
		return err
	}

	return nil
}
