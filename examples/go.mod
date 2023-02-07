module github.com/ServiceWeaver/weaver/examples

go 1.19

replace github.com/ServiceWeaver/weaver => ../

replace github.com/ServiceWeaver/weaver/gke => ../gke/

replace github.com/ServiceWeaver/weaver/dolt => ../dolt

require (
	cloud.google.com/go/compute/metadata v0.2.1
	github.com/ServiceWeaver/weaver v0.0.0
	github.com/dolthub/go-mysql-server v0.14.0
	github.com/durango/go-credit-card v0.0.0-20220404131259-a9e175ba4082
	github.com/go-sql-driver/mysql v1.6.0
	github.com/google/go-cmp v0.5.9
	github.com/google/uuid v1.3.0
	github.com/gorilla/mux v1.8.0
	github.com/hashicorp/golang-lru v0.5.4
	github.com/mattn/go-sqlite3 v1.14.16
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.32.0
	go.opentelemetry.io/otel v1.11.1
	go.opentelemetry.io/otel/trace v1.11.1
	golang.org/x/exp v0.0.0-20221012211006-4de253d81b95
	golang.org/x/image v0.0.0-20190802002840-cff245a6509b
)

require (
	cloud.google.com/go/compute v1.12.1 // indirect
	github.com/BurntSushi/toml v1.2.0 // indirect
	github.com/DataDog/hyperloglog v0.0.0-20220214164406-974598347557 // indirect
	github.com/antlr/antlr4/runtime/Go/antlr v0.0.0-20220418222510-f25a4f6275ed // indirect
	github.com/cespare/xxhash v1.1.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/dolthub/vitess v0.0.0-20221031111135-9aad77e7b39f // indirect
	github.com/felixge/httpsnoop v1.0.2 // indirect
	github.com/fsnotify/fsnotify v1.5.4 // indirect
	github.com/go-kit/kit v0.10.0 // indirect
	github.com/go-logr/logr v1.2.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/gocraft/dbr/v2 v2.7.2 // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/google/cel-go v0.12.5 // indirect
	github.com/google/flatbuffers v2.0.6+incompatible // indirect
	github.com/google/pprof v0.0.0-20221010195024-131d412537ea // indirect
	github.com/lestrrat-go/strftime v1.0.4 // indirect
	github.com/lightstep/varopt v1.3.0 // indirect
	github.com/mitchellh/hashstructure v1.1.0 // indirect
	github.com/oliveagle/jsonpath v0.0.0-20180606110733-2e52cf6e6852 // indirect
	github.com/pkg/browser v0.0.0-20210911075715-681adbf594b8 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/shopspring/decimal v1.2.0 // indirect
	github.com/sirupsen/logrus v1.8.1 // indirect
	github.com/stoewer/go-strcase v1.2.0 // indirect
	github.com/stretchr/testify v1.8.0 // indirect
	go.opentelemetry.io/otel/exporters/stdout/stdouttrace v1.7.0 // indirect
	go.opentelemetry.io/otel/metric v0.30.0 // indirect
	go.opentelemetry.io/otel/sdk v1.11.1 // indirect
	golang.org/x/mod v0.6.0-dev.0.20220419223038-86c51ed26bb4 // indirect
	golang.org/x/sync v0.1.0 // indirect
	golang.org/x/sys v0.0.0-20220919091848-fb04ddd9f9c8 // indirect
	golang.org/x/term v0.0.0-20210927222741-03fcf44c2211 // indirect
	golang.org/x/text v0.4.0 // indirect
	golang.org/x/tools v0.1.12 // indirect
	google.golang.org/genproto v0.0.0-20221109142239-94d6d90a7d66 // indirect
	google.golang.org/grpc v1.50.1 // indirect
	google.golang.org/protobuf v1.28.1 // indirect
	gopkg.in/src-d/go-errors.v1 v1.0.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
