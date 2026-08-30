.PHONY: build test run run-workload run-reads run-writes run-bufferpool run-sync clean

BINARY := bin/indexlab

# Experiment run parameters. Override on the command line, e.g.:
#   make run N=20000 POOL_CAPACITY=32
N             ?= 5000
POOL_CAPACITY ?= 64
SYNC_INTERVAL ?= 100

build:
	go build -o $(BINARY) ./cmd/indexlab 

test:
	go test ./...

run: build
	./$(BINARY) -run=all -n=$(N) -pool-capacity=$(POOL_CAPACITY) -sync-interval=$(SYNC_INTERVAL)

run-workload: build
	./$(BINARY) -run=workload -n=$(N)

run-reads: build
	./$(BINARY) -run=reads -n=$(N)

run-writes: build
	./$(BINARY) -run=writes -n=$(N)

run-bufferpool: build
	./$(BINARY) -run=bufferpool -n=$(N) -pool-capacity=$(POOL_CAPACITY)

run-sync: build
	./$(BINARY) -run=sync -n=$(N) -sync-interval=$(SYNC_INTERVAL)

clean:
	rm -rf bin