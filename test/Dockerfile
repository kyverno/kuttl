FROM kudobuilder/golang:1.18

WORKDIR $GOPATH/src/github.com/kyverno/kuttl

# install docker
RUN apt-get update && apt-get install -y \
    apt-transport-https \
    ca-certificates \
    curl \
    gnupg2 \
    software-properties-common && curl -fsSL https://download.docker.com/linux/debian/gpg | apt-key add - && \
    add-apt-repository "deb [arch=amd64] https://download.docker.com/linux/debian $(lsb_release -cs) stable" && \
    apt-get update && apt-get install -y docker-ce-cli

COPY Makefile go.mod go.sum ./
RUN make download
COPY pkg/ pkg/
COPY cmd/ cmd/
COPY hack/run-integration-tests.sh hack/run-integration-tests.sh
COPY hack/run-e2e-tests.sh hack/run-e2e-tests.sh
COPY test/ test/
COPY kuttl-test.yaml kuttl-test.yaml
