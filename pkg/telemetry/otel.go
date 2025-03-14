package telemetry

import (
	"context"
	"errors"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutlog"
	"go.opentelemetry.io/otel/sdk/resource"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"

	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

type ExporterType int

const (
	GrpcExporter ExporterType = iota
	HttpExporter
	StdoutExporter
)

// SetupOTelSDK bootstraps the OpenTelemetry pipeline.
// If it does not return an error, make sure to call shutdown for proper cleanup.
func SetupOTelSDK(ctx context.Context, exporterType ExporterType, otlpAddress string, resources *resource.Resource) (shutdown func(context.Context) error, err error) {
	var shutdownFuncs []func(context.Context) error

	// shutdown calls cleanup functions registered via shutdownFuncs.
	// The errors from the calls are joined.
	// Each registered cleanup will be invoked once.
	shutdown = func(ctx context.Context) error {
		var err error
		for _, fn := range shutdownFuncs {
			err = errors.Join(err, fn(ctx))
		}
		shutdownFuncs = nil
		return err
	}

	// handleErr calls shutdown for cleanup and makes sure that all errors are returned.
	handleErr := func(inErr error) {
		err = errors.Join(inErr, shutdown(ctx))
	}

	// Set up propagator.
	prop := newPropagator()
	otel.SetTextMapPropagator(prop)

	// Set up trace provider.
	tracerProvider, err := newTracerProvider(ctx, exporterType, otlpAddress, resources)
	if err != nil {
		handleErr(err)
		return
	}
	shutdownFuncs = append(shutdownFuncs, tracerProvider.Shutdown)
	otel.SetTracerProvider(tracerProvider)

	// Set up meter provider.
	meterProvider, err := newMeterProvider(ctx, exporterType, otlpAddress, resources)
	if err != nil {
		handleErr(err)
		return
	}
	shutdownFuncs = append(shutdownFuncs, meterProvider.Shutdown)
	otel.SetMeterProvider(meterProvider)

	// Set up logger provider.
	loggerProvider, err := newLoggerProvider(ctx, exporterType, otlpAddress, resources)
	if err != nil {
		handleErr(err)
		return
	}
	shutdownFuncs = append(shutdownFuncs, loggerProvider.Shutdown)
	global.SetLoggerProvider(loggerProvider)

	return
}

func newPropagator() propagation.TextMapPropagator {
	return propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
}

func newTracerProvider(ctx context.Context, exporterType ExporterType, otlpAddress string, resources *resource.Resource) (*sdktrace.TracerProvider, error) {
	var err error
	var traceExporter sdktrace.SpanExporter

	switch exporterType {
	case GrpcExporter:
		traceExporter, err = otlptracegrpc.New(ctx,
			otlptracegrpc.WithInsecure(),
			otlptracegrpc.WithEndpoint(otlpAddress),
		)
	case HttpExporter:
		traceExporter, err = otlptracehttp.New(ctx,
			otlptracehttp.WithInsecure(),
			otlptracehttp.WithEndpoint(otlpAddress),
		)
	case StdoutExporter:
		traceExporter, err = stdouttrace.New(
			stdouttrace.WithPrettyPrint())
	}

	if err != nil {
		return nil, err
	}

	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter,
			// Default is 5s. Set to 1s for demonstrative purposes.
			sdktrace.WithBatchTimeout(time.Second)),
		sdktrace.WithResource(resources),
	)
	return tracerProvider, nil
}

func newMeterProvider(ctx context.Context, exporterType ExporterType, otlpAddress string, resources *resource.Resource) (*sdkmetric.MeterProvider, error) {
	var err error
	var metricExporter sdkmetric.Exporter

	switch exporterType {
	case GrpcExporter:
		metricExporter, err = otlpmetricgrpc.New(ctx,
			otlpmetricgrpc.WithInsecure(),
			otlpmetricgrpc.WithEndpoint(otlpAddress))
	case HttpExporter:
		metricExporter, err = otlpmetrichttp.New(ctx,
			otlpmetrichttp.WithInsecure(),
			otlpmetrichttp.WithEndpoint(otlpAddress))
	case StdoutExporter:
		metricExporter, err = stdoutmetric.New()
	}

	if err != nil {
		return nil, err
	}

	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter,
			// Default is 1m. Set to 3s for demonstrative purposes.
			sdkmetric.WithInterval(3*time.Second))),
		sdkmetric.WithResource(resources),
	)
	return meterProvider, nil
}

func newLoggerProvider(ctx context.Context, exporterType ExporterType, otlpAddress string, resources *resource.Resource) (*sdklog.LoggerProvider, error) {
	var err error
	var logExporter sdklog.Exporter

	switch exporterType {
	case GrpcExporter:
		logExporter, err = otlploggrpc.New(nil,
			otlploggrpc.WithInsecure(),
			otlploggrpc.WithEndpoint(otlpAddress),
		)
	case HttpExporter:
		logExporter, err = otlploghttp.New(nil,
			otlploghttp.WithInsecure(),
			otlploghttp.WithEndpoint(otlpAddress),
		)
	case StdoutExporter:
		logExporter, err = stdoutlog.New()
	}

	if err != nil {
		return nil, err
	}

	loggerProvider := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewBatchProcessor(logExporter)),
		sdklog.WithResource(resources),
	)
	return loggerProvider, nil
}
