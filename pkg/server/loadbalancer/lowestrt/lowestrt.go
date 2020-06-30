package lowestrt

import (
	"fmt"
	"github.com/containous/traefik/v2/pkg/config/dynamic"
	"github.com/vulcand/oxy/roundrobin"
	"github.com/vulcand/oxy/utils"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// TODO

type server struct {
	url *url.URL
}

type LrtBalancer struct {
	fwd http.Handler
	cfg *dynamic.LowestResponseTime

	mutex   *sync.Mutex
	servers []*server

	log *log.Logger
}

func New(fwd http.Handler, cfg *dynamic.LowestResponseTime) (*LrtBalancer, error) {
	return &LrtBalancer{
		fwd: fwd,
		cfg: cfg,

		mutex:   &sync.Mutex{},
		servers: []*server{},
	}, nil

}

func (lb *LrtBalancer) NextServer() *server {
	return lb.servers[0]
}

func (lb *LrtBalancer) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	s := lb.NextServer()

	then := time.Now()

	// FIXME: just a prototype impl without sticky sessions

	newReq := *req
	newReq.URL = s.url

	lb.fwd.ServeHTTP(w, &newReq)

	duration := time.Now().Sub(then).Milliseconds()
	// serving by "lowest response time" requires information about the response time, which can only be
	// obtained by sending requests to the server in the first place. so it will be necessary to occasionally
	// send requests to other servers.
	// oxy's Rebalancer seems to generalize this by adapting weights, which may simplify balancing.
	log.Printf("serving on %s took %d ms\n", s.url, duration)
}

func (lb *LrtBalancer) Servers() []*url.URL {
	lb.mutex.Lock()
	defer lb.mutex.Unlock()

	out := make([]*url.URL, len(lb.servers))
	for i, s := range lb.servers {
		out[i] = s.url
	}
	return out
}

func (lb *LrtBalancer) RemoveServer(u *url.URL) error {
	lb.mutex.Lock()
	defer lb.mutex.Unlock()

	e, index := lb.findServerByURL(u)
	if e == nil {
		return fmt.Errorf("server not found")
	}

	lb.servers = append(lb.servers[:index], lb.servers[index+1:]...)
	return nil
}

// UpsertServer In case if server is already present in the load balancer, returns error
func (lb *LrtBalancer) UpsertServer(u *url.URL, _ ...roundrobin.ServerOption) error {
	lb.mutex.Lock()
	defer lb.mutex.Unlock()

	if u == nil {
		return fmt.Errorf("server URL can't be nil")
	}

	if s, _ := lb.findServerByURL(u); s != nil {
		return nil
	}

	srv := &server{url: utils.CopyURL(u)}

	lb.servers = append(lb.servers, srv)
	return nil
}

func (lb *LrtBalancer) findServerByURL(u *url.URL) (*server, int) {
	if len(lb.servers) == 0 {
		return nil, -1
	}
	for i, s := range lb.servers {
		if urlEquals(u, s.url) {
			return s, i
		}
	}
	return nil, -1
}

func urlEquals(a, b *url.URL) bool {
	return a.Path == b.Path && a.Host == b.Host && a.Scheme == b.Scheme
}
