package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/billmeyer/go-otel-core/pkg/app"
	"github.com/billmeyer/go-otel-core/pkg/telemetry"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

const (
	serviceName           = "rolldice.service"
	serviceVersion        = "0.1.0"
	deploymentEnvironment = "dev"
	exporterType          = telemetry.StdoutExporter

	// otlpAddress set to be the hostname:portnumber of the collector to send telemetry to.
	// Ignored when exporterType == StdoutExporter
	// Ensure the port number matches the protocol being used:
	//	gRPC defaults to port 4317
	//	HTTP defaults to port 4318
	otlpAddress = "localhost:4317"
)

func main() {
	if err := run(); err != nil {
		log.Fatalln(err)
	}
}

func run() (err error) {
	// Handle SIGINT (CTRL+C) gracefully.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	resources, err := sdkresource.New(ctx,
		sdkresource.WithAttributes(
			semconv.ServiceNameKey.String(serviceName),
			semconv.ServiceVersionKey.String(serviceVersion),
			semconv.DeploymentEnvironment(deploymentEnvironment)),
		sdkresource.WithSchemaURL(semconv.SchemaURL),
		sdkresource.WithFromEnv(), // pull attributes from OTEL_RESOURCE_ATTRIBUTES and OTEL_SERVICE_NAME environment variables
		sdkresource.WithProcess(), // This option configures a set of Detectors that discover process information
		sdkresource.WithOS(),      // This option configures a set of Detectors that discover OS information
		//sdkresource.WithContainer(), // This option configures a set of Detectors that discover container information
		sdkresource.WithHost(), // This option configures a set of Detectors that discover host information
	)
	if err != nil {
		fmt.Printf("failed to create resource: %w", err)
		return err
	}

	// Set up OpenTelemetry.
	otelShutdown, err := telemetry.SetupOTelSDK(ctx, exporterType, otlpAddress, resources)
	if err != nil {
		return
	}
	// Handle shutdown properly so nothing leaks.
	defer func() {
		err = errors.Join(err, otelShutdown(context.Background()))
	}()

	// Start HTTP server.
	srv := &http.Server{
		Addr:         ":8080",
		BaseContext:  func(_ net.Listener) context.Context { return ctx },
		ReadTimeout:  time.Second,
		WriteTimeout: 10 * time.Second,
		Handler:      newHTTPHandler(),
	}
	srvErr := make(chan error, 1)
	go func() {
		srvErr <- srv.ListenAndServe()
	}()

	// Wait for interruption.
	select {
	case err = <-srvErr:
		// Error when starting HTTP server.
		return
	case <-ctx.Done():
		// Wait for first CTRL+C.
		// Stop receiving signal notifications as soon as possible.
		stop()
	}

	// When Shutdown is called, ListenAndServe immediately returns ErrServerClosed.
	err = srv.Shutdown(context.Background())
	return
}

func newHTTPHandler() http.Handler {
	mux := http.NewServeMux()

	// handleFunc is a replacement for mux.HandleFunc
	// which enriches the handler's HTTP instrumentation with the pattern as the http.route.
	handleFunc := func(pattern string, handlerFunc func(http.ResponseWriter, *http.Request)) {
		// Configure the "http.route" for the HTTP instrumentation.
		handler := otelhttp.WithRouteTag(pattern, http.HandlerFunc(handlerFunc))
		mux.Handle(pattern, handler)
	}

	// Register handlers.
	handleFunc("/rolldice/", app.Rolldice)
	handleFunc("/rolldice/{player}", app.Rolldice)

	// Add HTTP instrumentation for the whole server.
	handler := otelhttp.NewHandler(mux, "/")
	return handler
}
