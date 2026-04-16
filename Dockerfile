FROM golang:1.26@sha256:595c7847cff97c9a9e76f015083c481d26078f961c9c8dca3923132f51fe12f1 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY main.go .
COPY pkg ./pkg/
COPY internal ./internal/

# Statically comple with CGO enabled will be needed if we integrate go-tree-sitter
# RUN GOOS=linux go build  --ldflags '-extldflags "-static"' -v -o codeowners main.go

# Statically compile with CGO disabled
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -v -o codeowners main.go

FROM alpine:latest@sha256:5b10f432ef3da1b8d4c7eb6c487f2f5a8f096bc91145e68878dd4a5019afde11

RUN apk update && apk add git

COPY --from=builder /app/codeowners /codeowners
COPY entrypoint.sh /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]
