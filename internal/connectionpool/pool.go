package connectionpool

import (
	"bufio"
	"bytes"
	"github.com/dgraph-io/ristretto"
	"io/ioutil"
	"log"
	"math"
	"math/rand"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"
)

type pool struct {
	connections     chan *connection
	healthChecks    map[string]*healthChecker
	client          *http.Client
	connsPerBackend int
	cacheEnabled    bool
	cache           *ristretto.Cache
}

//Exported method for creation of a connection-pool takes []string
//ex: ['http://localhost:9000','http://localhost:9000']
func New(c *Config) *pool {
	backends := c.Backends
	connsPerBackend := c.NumConns
	cacheEnabled := c.EnableCache

	var maxRequests int
	backendCount := int(math.Max(float64(len(backends)), float64(1)))

	if backendCount > 0 {
		maxRequests = connsPerBackend * backendCount * 2
	}

	tr := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: 10 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 10 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		MaxIdleConns:          maxRequests + backendCount,
		IdleConnTimeout:       120 * time.Second,
		MaxConnsPerHost:       connsPerBackend + 1,
		MaxIdleConnsPerHost:   connsPerBackend + 1,
	}

	client := &http.Client{
		Timeout:   30 * time.Second,
		Transport: tr,
	}

	var cache *ristretto.Cache
	var err error
	if cache, err = buildCache(); err != nil {
		log.Printf("Error creating cache: %v", err)
		cacheEnabled = false
	}

	connectionPool := &pool{
		connections:     make(chan *connection, maxRequests),
		healthChecks:    make(map[string]*healthChecker),
		client:          client,
		connsPerBackend: connsPerBackend,
		cache:           cache,
		cacheEnabled:    cacheEnabled,
	}

	poolConnections := []*connection{}
	for _, backend := range backends {
		poolConnections = connectionPool.addBackend(poolConnections, backend)
	}

	shuffle(poolConnections, connectionPool.connections)
	go connectionPool.ListenForBackendChanges()

	return connectionPool
}

func shuffle(conns []*connection, ch chan *connection) {
	rand.Seed(time.Now().UnixNano())
	rand.Shuffle(len(conns), func(i, j int) {
		conns[i], conns[j] = conns[j], conns[i]
	})

	for _, conn := range conns {
		ch <- conn
	}
}

func buildCache() (*ristretto.Cache, error) {
	cache, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: 1e7,     // number of keys to track frequency of (10M).
		MaxCost:     1 << 30, // maximum cost of cache (1GB).
		BufferItems: 64,      // number of keys per Get buffer.
	})

	return cache, err
}

//Exported method for passing a request to a connection from the pool
//Returns a 503 status code if request is unsuccessful
func (p *pool) Fetch(w http.ResponseWriter, r *http.Request) {
	select {
	case connection := <-p.connections:
		defer func() {
			p.connections <- connection
		}()

		err := connection.get(w, r)
		if err != nil {
			log.Printf("retrying err with request: %s", err.Error())
			p.Fetch(w, r)
		}
	default:
		w.WriteHeader(http.StatusServiceUnavailable)
	}
}

func (p *pool) Shutdown() {
	for _, hc := range p.healthChecks {
		hc.Shutdown()
	}
}

func (p *pool) ListenForBackendChanges() {
	const SockAddr = "/tmp/goaround.sock"

	if err := os.RemoveAll(SockAddr); err != nil {
		log.Fatal(err)
	}

	l, err := net.Listen("unix", SockAddr)
	if err != nil {
		log.Fatal("listen error:", err)
	}
	defer l.Close()

	for {
		conn, err := l.Accept()
		if err != nil {
			log.Fatal("accept error:", err)
		}

		scanner := bufio.NewScanner(conn)
		for scanner.Scan() {
			updated := strings.Split(scanner.Text(), ",")

			var currentBackends []string
			for k := range p.healthChecks {
				currentBackends = append(currentBackends, k)
			}

			added, removed := difference(currentBackends, updated)
			log.Printf("Adding: %s", added)
			log.Printf("Removing: %s", removed)

			for _, removedBackend := range removed {
				if len(added) > 0 {
					var new string
					new, added = added[0], added[1:]
					url, err := url.ParseRequestURI(new)
					if err != nil {
						log.Printf("Error adding backend, %s", new)
					} else {
						proxy := httputil.NewSingleHostReverseProxy(url)
						proxy.Transport = p.client.Transport

						if p.cacheEnabled {
							cacheResponse := func(r *http.Response) error {
								body, err := ioutil.ReadAll(r.Body)
								cacheable := string(body)
								r.Body = ioutil.NopCloser(bytes.NewBuffer(body))

								path := r.Request.URL.Path
								if err == nil {
									p.cache.Set(path, cacheable, 1)
								}

								return nil
							}
							proxy.ModifyResponse = cacheResponse
						}

						newHC := p.healthChecks[removedBackend].Reuse(new, proxy)
						p.healthChecks[new] = newHC
					}
				} else {
					p.healthChecks[removedBackend].Shutdown()
				}

				delete(p.healthChecks, removedBackend)
			}

			poolConnections := []*connection{}
			for _, addedBackend := range added {
				poolConnections = p.addBackend(poolConnections, addedBackend)
			}

			shuffle(poolConnections, p.connections)
		}
	}
}

func difference(original []string, updated []string) (added []string, removed []string) {
	oldBackends := make(map[string]bool)
	for _, i := range original {
		oldBackends[i] = true
	}

	newBackends := make(map[string]bool)
	for _, i := range updated {
		newBackends[i] = true
	}

	for _, i := range updated {
		if _, ok := oldBackends[i]; !ok {
			added = append(added, i)
		}
	}

	for _, i := range original {
		if _, ok := newBackends[i]; !ok {
			removed = append(removed, i)
		}
	}

	return
}

func (p *pool) addBackend(connections []*connection, backend string) []*connection {
	url, err := url.ParseRequestURI(backend)
	if err != nil {
		log.Printf("error parsing backend url: %s", backend)
	} else {
		proxy := httputil.NewSingleHostReverseProxy(url)
		proxy.Transport = p.client.Transport

		if p.cacheEnabled {
			cacheResponse := func(r *http.Response) error {
				body, err := ioutil.ReadAll(r.Body)
				cacheable := string(body)
				r.Body = ioutil.NopCloser(bytes.NewBuffer(body))

				path := r.Request.URL.Path
				if err == nil {
					p.cache.Set(path, cacheable, 1)
				}

				return nil
			}

			proxy.ModifyResponse = cacheResponse
		}

		configuredConn, err := newConnection(proxy, backend, p.cache)
		if err != nil {
			log.Printf("Error adding connection for: %s", backend)
		} else {
			backendConnections := make([]chan message, p.connsPerBackend)

			for i := 0; i < p.connsPerBackend; i++ {
				connections = append(connections, configuredConn)
				backendConnections[i] = configuredConn.messages
			}

			hc := &healthChecker{
				client:      p.client,
				subscribers: backendConnections,
				backend:     configuredConn.backend,
				done:        make(chan bool, 1),
			}

			p.healthChecks[backend] = hc
			go hc.Start()
		}
	}

	return connections
}
