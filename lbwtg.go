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

// LoadBalancer represents a round-robin load balancer with health checks for multiple target groups
type LoadBalancer struct {
	targetGroups []*TargetGroup
	mu           sync.Mutex
}

// TargetGroup represents a group of backend servers for a specific URI path
type TargetGroup struct {
	URIPath string
	Servers []*Server
}

// NewLoadBalancer creates a new LoadBalancer with a list of target groups
func NewLoadBalancer(targetGroups []*TargetGroup) *LoadBalancer {
	return &LoadBalancer{targetGroups: targetGroups}
}

// ServeHTTP handles incoming HTTP requests and forwards them to healthy backend servers
func (lb *LoadBalancer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	for _, targetGroup := range lb.targetGroups {
		if r.URL.Path == targetGroup.URIPath {
			server := lb.getNextServer(targetGroup)
			if server != nil && lb.isServerHealthy(server) {
				// Create a reverse proxy
				proxy := httputil.NewSingleHostReverseProxy(server.URL)

				// Update the request to preserve the original URL path
				r.URL.Path = fmt.Sprintf("/%s%s", server.URL.Host, r.URL.Path)

				// Forward the request to the healthy backend server
				proxy.ServeHTTP(w, r)
				return
			}
		}
	}

	http.Error(w, "No healthy backend servers available", http.StatusServiceUnavailable)
}

// getNextServer returns the next server in the round-robin order for a given target group
func (lb *LoadBalancer) getNextServer(targetGroup *TargetGroup) *Server {
	serverCount := len(targetGroup.Servers)
	if serverCount == 0 {
		return nil
	}

	// Round-robin index for the target group
	targetGroupIndex := len(targetGroup.Servers) % serverCount

	// Update the round-robin index for the next request
	targetGroupIndex = (targetGroupIndex + 1) % serverCount

	return targetGroup.Servers[targetGroupIndex]
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
	// Define target groups with different URI paths and backend servers
	targetGroups := []*TargetGroup{
		{
			URIPath: "/app1",
			Servers: []*Server{
				{URL: parseURL("http://localhost:8081"), healthCheckPath: "/health"},
				{URL: parseURL("http://localhost:8082"), healthCheckPath: "/health"},
			},
		},
		{
			URIPath: "/app2",
			Servers: []*Server{
				{URL: parseURL("http://localhost:8083"), healthCheckPath: "/health"},
				{URL: parseURL("http://localhost:8084"), healthCheckPath: "/health"},
			},
		},
	}

	// Create a new load balancer with target groups
	loadBalancer := NewLoadBalancer(targetGroups)

	// Set up the HTTP server
	http.HandleFunc("/", loadBalancer.ServeHTTP)
	fmt.Println("Load balancer listening on :8080")
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		panic(err)
	}
}

// Helper function to parse a URL and panic on error
func parseURL(urlStr string) *url.URL {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		panic(err)
	}
	return parsedURL
}
