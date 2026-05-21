package k8s

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"sync"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

// PortMapping maps a local port to a remote container port.
type PortMapping struct {
	LocalPort  int
	RemotePort int
}

// Forwarder owns a running port-forward session.
type Forwarder struct {
	stop     chan struct{}
	stopOnce sync.Once
	done     chan struct{}
	mu       sync.RWMutex
	err      error
}

// Ready blocks until the forwarder is accepting connections on all local ports
// or ctx is cancelled.
func (f *Forwarder) Ready(ctx context.Context, ready <-chan struct{}) error {
	select {
	case <-ready:
		return nil
	case <-f.done:
		if err := f.Err(); err != nil {
			return err
		}
		return fmt.Errorf("port-forward exited before becoming ready")
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Close stops the forwarder. Returns the final error from the port-forward
// goroutine (usually nil).
func (f *Forwarder) Close() error {
	f.stopOnce.Do(func() { close(f.stop) })
	<-f.done
	return f.Err()
}

func (f *Forwarder) Err() error {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.err
}

func (f *Forwarder) finish(err error) {
	f.mu.Lock()
	f.err = err
	f.mu.Unlock()
	close(f.done)
}

// StartPortForward opens a port-forward to the given pod. Returns the forwarder
// and a channel that closes when forwarding is ready. The caller must call
// Close() to stop.
func StartPortForward(restCfg *rest.Config, namespace, pod string, ports []PortMapping, errOut, infoOut io.Writer) (*Forwarder, <-chan struct{}, error) {
	if len(ports) == 0 {
		return nil, nil, fmt.Errorf("no ports to forward")
	}
	cs, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, nil, fmt.Errorf("build kube client: %w", err)
	}

	req := cs.CoreV1().RESTClient().
		Post().
		Resource("pods").
		Namespace(namespace).
		Name(pod).
		SubResource("portforward")

	transport, upgrader, err := spdy.RoundTripperFor(restCfg)
	if err != nil {
		return nil, nil, fmt.Errorf("spdy transport: %w", err)
	}
	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, "POST", mustURL(req.URL().String()))

	specs := make([]string, 0, len(ports))
	for _, p := range ports {
		specs = append(specs, strconv.Itoa(p.LocalPort)+":"+strconv.Itoa(p.RemotePort))
	}

	stop := make(chan struct{})
	ready := make(chan struct{})

	pf, err := portforward.New(dialer, specs, stop, ready, infoOut, errOut)
	if err != nil {
		return nil, nil, fmt.Errorf("create port-forward: %w", err)
	}

	fwd := &Forwarder{stop: stop, done: make(chan struct{})}
	go func() {
		fwd.finish(pf.ForwardPorts())
	}()

	return fwd, ready, nil
}

func mustURL(raw string) *url.URL {
	u, err := url.Parse(raw)
	if err != nil {
		panic(fmt.Errorf("portforward: invalid URL %q: %w", raw, err))
	}
	return u
}
