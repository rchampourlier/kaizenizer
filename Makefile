GO_BUILD_ENV := CGO_ENABLED=0 GOOS=linux GOARCH=amd64
CMD_NAME=kaizenizer-metrics
DOCKER_BUILD=$(shell pwd)/.docker_build
DOCKER_CMD=$(DOCKER_BUILD)/$(CMD_NAME)
DOCKER_TAG=latest

$(DOCKER_CMD): clean
	mkdir -p $(DOCKER_BUILD)
	$(GO_BUILD_ENV) go build -v -o $(DOCKER_CMD) .
	docker build . -t $(DOCKER_TAG)

push:
	docker push $(ECR_TAG)

container: $(DOCKER_CMD)
	docker build . -t $(CMD_NAME):$(DOCKER_TAG)

clean:
	rm -rf $(DOCKER_BUILD)

test:
	go test -v -covermode=count -coverprofile=coverage.out ./...
