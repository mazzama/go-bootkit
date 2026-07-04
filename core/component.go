// Package core provides the foundational interfaces and lifecycle management
// for Go Boot Kit applications.
//
// # Component Interface
//
// The Component interface defines the contract for all service components:
//
//	type Component interface {
//	    Name() string
//	    Start(ctx context.Context) error
//	    Stop(ctx context.Context) error
//	}
//
// # Readyable Interface
//
// Components that have a startup/ready signal should implement the Readyable
// interface, which provides a channel that closes when the component is ready
// to serve traffic.
//
// # Application Runner
//
// The ApplicationRunner orchestrates component lifecycle, managing concurrent
// startup, graceful shutdown, and optional start deadline enforcement.
//
// Example:
//
//	ctx := context.Background()
//	runner := NewApplicationRunner(
//	    WithServices(server, database, cache),
//	    WithShutdownTimeout(30*time.Second),
//	)
//	if err := runner.Run(ctx); err != nil {
//	    log.Fatal(err)
//	 }
package core

import "context"

// Component defines the lifecycle contract for all service components.
//
// All components in Go Boot Kit must implement this interface to be managed
// by the ApplicationRunner. The component lifecycle consists of three phases:
//
//   - Start: Called when the application starts. Should block until the component
//     is stopped or the context is canceled.
//   - Running: The component performs its work.
//   - Stop: Called during graceful shutdown. Should clean up resources and exit.
//
// The Start method should block until the context is canceled, at which point
// it should initiate cleanup and return. If Start returns a non-nil error
// (other than context.Canceled), the ApplicationRunner will shut down all
// components and return the error.
//
// Example:
//
//	type MyComponent struct{}
//
//	func (m *MyComponent) Name() string { return "my-component" }
//
//	func (m *MyComponent) Start(ctx context.Context) error {
//	    <-ctx.Done()
//	    return nil
//	}
//
//	func (m *MyComponent) Stop(ctx context.Context) error {
//	    // cleanup resources
//	    return nil
//	}
type Component interface {
	// Name returns the component's unique identifier.
	//
	// This name is used for logging and error messages. It should be unique
	// across all components in the application.
	Name() string

	// Start begins the component's operation.
	//
	// Start should block until the provided context is canceled. When the
	// context is canceled, the component should perform any necessary cleanup
	// and return. If a non-nil error (other than context.Canceled) is returned,
	// the ApplicationRunner will shut down all components.
	//
	// The context passed to Start is canceled when the application receives
	// a termination signal (SIGINT/SIGTERM) or when another component returns
	// an error.
	Start(ctx context.Context) error

	// Stop gracefully shuts down the component.
	//
	// Stop is called with a separate context that has a timeout deadline
	// (configurable via WithShutdownTimeout). Components should attempt to
	// complete in-flight work and release resources within this deadline.
	//
	// If the timeout is exceeded, the ApplicationRunner will continue the
	// shutdown process without waiting for this component to finish.
	Stop(ctx context.Context) error
}

// Readyable is an optional interface that components can implement to signal
// when they are ready to serve traffic.
//
// Components that have a startup period (e.g., servers waiting for bind,
// databases connecting) should implement this interface. The ApplicationRunner
// can enforce a start deadline that waits for all Readyable components to
// close their ready channel before considering the application started.
//
// The Ready() method returns a read-only channel that closes when the component
// is ready. Components should close this channel themselves, typically in a
// separate goroutine that performs startup checks.
//
// Example:
//
//	func (s *MyServer) Ready() <-chan struct{} {
//	    return s.readyCh
//	}
//
//	func (s *MyServer) Start(ctx context.Context) error {
//	    go func() {
//	        // perform startup
//	        close(s.readyCh)
//	    }()
//	    <-ctx.Done()
//	    return nil
//	}
type Readyable interface {
	// Ready returns a channel that closes when the component is ready.
	//
	// The returned channel must be closed by the component when it has
	// completed its startup sequence and is ready to serve traffic.
	// If the component is always ready immediately, it may close the
	// channel in its Start method before blocking.
	//
	// If this method returns nil, the component is considered immediately ready.
	Ready() <-chan struct{}
}
