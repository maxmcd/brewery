package tracing

import (
	"context"
	"log"
	"os"
	"sync"
	"time"

	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.20.0"
	"go.opentelemetry.io/otel/trace"
)

var lock sync.Mutex
var tps []*sdktrace.TracerProvider

// Init returns an instance of Jaeger Tracer.
func Init(service string) trace.Tracer {
	os.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:4317")
	os.Setenv("OTEL_EXPORTER_OTLP_INSECURE", "true")
	client := otlptracegrpc.NewClient(
		otlptracegrpc.WithInsecure(),
	)
	exporter, err := otlptrace.New(context.Background(), client)
	if err != nil {
		log.Fatal("creating OTLP trace exporter: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(newResource(service)),
	)
	lock.Lock()
	tps = append(tps, tp)
	lock.Unlock()

	return tp.Tracer(service)
}

func newResource(service string) *resource.Resource {
	return resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(service),
		semconv.ServiceVersion("0.0.1"),
	)
}

func Stop() {
	lock.Lock()
	if len(tps) == 0 {
		return
	}
	for _, tp := range tps {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*1)
		defer cancel()

		if err := tp.Shutdown(ctx); err != nil {
			panic(err)
		}
	}
}
