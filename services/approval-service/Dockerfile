FROM golang:1.22-alpine AS build
WORKDIR /src

COPY shared/go.mod shared/go.sum ./shared/
COPY services/approval-service/go.mod services/approval-service/go.sum ./services/approval-service/

RUN cd services/approval-service && go mod download

COPY shared ./shared
COPY services/approval-service ./services/approval-service

RUN cd services/approval-service && CGO_ENABLED=0 GOOS=linux go build -o /out/approval-service ./cmd/server

FROM gcr.io/distroless/base-debian12:nonroot
WORKDIR /
COPY --from=build /out/approval-service /approval-service
EXPOSE 8003
USER nonroot:nonroot
ENTRYPOINT ["/approval-service"]
