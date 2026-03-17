FROM golang:1.26@sha256:c42e4d75186af6a44eb4159dcfac758ef1c05a7011a0052fe8a8df016d8e8fb9 AS builder

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

FROM alpine:latest@sha256:25109184c71bdad752c8312a8623239686a9a2071e8825f20acb8f2198c3f659

RUN apk update && apk add git

COPY --from=builder /app/codeowners /codeowners
COPY entrypoint.sh /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]
