module github.com/ServiceWeaver/weaver/examples

go 1.19

replace github.com/ServiceWeaver/weaver => ../

require (
	github.com/ServiceWeaver/weaver v0.0.0-00010101000000-000000000000
	github.com/go-sql-driver/mysql v1.7.0
	github.com/google/go-cmp v0.5.9
	github.com/google/uuid v1.3.0
	github.com/gorilla/mux v1.8.0
	github.com/hashicorp/golang-lru v0.5.4
	github.com/mattn/go-sqlite3 v1.14.16
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.39.0
	go.opentelemetry.io/otel v1.13.0
	go.opentelemetry.io/otel/trace v1.13.0
	golang.org/x/exp v0.0.0-20230206171751-46f607a40771
	golang.org/x/image v0.3.0
)

require (
	github.com/BurntSushi/toml v1.2.0 // indirect
	github.com/DataDog/hyperloglog v0.0.0-20220214164406-974598347557 // indirect
	github.com/antlr/antlr4/runtime/Go/antlr v0.0.0-20220418222510-f25a4f6275ed // indirect
	github.com/felixge/httpsnoop v1.0.3 // indirect
	github.com/fsnotify/fsnotify v1.5.4 // indirect
	github.com/go-logr/logr v1.2.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/google/cel-go v0.12.5 // indirect
	github.com/google/pprof v0.0.0-20221010195024-131d412537ea // indirect
	github.com/lightstep/varopt v1.3.0 // indirect
	github.com/pkg/browser v0.0.0-20210911075715-681adbf594b8 // indirect
	github.com/stoewer/go-strcase v1.2.0 // indirect
	go.opentelemetry.io/otel/exporters/stdout/stdouttrace v1.7.0 // indirect
	go.opentelemetry.io/otel/metric v0.36.0 // indirect
	go.opentelemetry.io/otel/sdk v1.11.1 // indirect
	golang.org/x/sys v0.1.0 // indirect
	golang.org/x/term v0.0.0-20210927222741-03fcf44c2211 // indirect
	golang.org/x/text v0.6.0 // indirect
	google.golang.org/genproto v0.0.0-20221109142239-94d6d90a7d66 // indirect
	google.golang.org/protobuf v1.28.1 // indirect
)
