package main

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"time"
)

// Server represents a backend server
type Server struct {
	URL             *url.URL
	healthCheckPath string
}

// LoadBalancer represents a round-robin load balancer with health checks
type LoadBalancer struct {
	servers []*Server
	index   int
	mu      sync.Mutex
}

// NewLoadBalancer creates a new LoadBalancer with a list of backend servers and a health check path
func NewLoadBalancer(serverURLs []string, healthCheckPath string) *LoadBalancer {
	var servers []*Server
	for _, urlStr := range serverURLs {
		serverURL, err := url.Parse(urlStr)
		if err != nil {
			panic(err)
		}
		servers = append(servers, &Server{URL: serverURL, healthCheckPath: healthCheckPath})
	}
	return &LoadBalancer{servers: servers, index: 0}
}

// ServeHTTP handles incoming HTTP requests and forwards them to healthy backend servers
func (lb *LoadBalancer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	for i := 0; i < len(lb.servers); i++ {
		server := lb.servers[lb.index]
		lb.index = (lb.index + 1) % len(lb.servers)

		// Perform health check with retries
		if lb.isServerHealthy(server) {
			// Create a reverse proxy
			proxy := httputil.NewSingleHostReverseProxy(server.URL)

			// Update the request to preserve the original URL path
			r.URL.Path = fmt.Sprintf("/%s%s", server.URL.Host, r.URL.Path)

			// Forward the request to the healthy backend server
			proxy.ServeHTTP(w, r)
			return
		}
	}
	http.Error(w, "No healthy backend servers available", http.StatusServiceUnavailable)
}

// isServerHealthy checks the health of a backend server with retries
func (lb *LoadBalancer) isServerHealthy(server *Server) bool {
	if server.healthCheckPath == "" {
		// If no health check path is specified, consider the server healthy
		return true
	}

	// Set a timeout for the health check
	client := http.Client{
		Timeout: time.Second * 5, // Adjust the timeout as needed
	}

	// Perform the health check with retries
	maxRetries := 3
	for retry := 0; retry < maxRetries; retry++ {
		resp, err := client.Get(server.URL.String() + server.healthCheckPath)
		if err != nil || resp.StatusCode != http.StatusOK {
			// Retry if the health check fails
			time.Sleep(time.Second) // Wait before the next retry
			continue
		}
		return true
	}

	// If all retries fail, consider the server unhealthy
	return false
}

func main() {
	// Define backend servers
	serverURLs := []string{"http://localhost:8081", "http://localhost:8082"}

	// Specify the health check path
	healthCheckPath := "/health"

	// Create a new load balancer with health check path
	loadBalancer := NewLoadBalancer(serverURLs, healthCheckPath)

	// Set up the HTTP server
	http.HandleFunc("/", loadBalancer.ServeHTTP)
	fmt.Println("Load balancer listening on :8080")
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		panic(err)
	}
}
