FROM golang:1.22-alpine AS build
WORKDIR /src

COPY go.work ./
COPY shared/go.mod shared/go.sum ./shared/
COPY services/audit-service/go.mod services/audit-service/go.sum ./services/audit-service/

RUN cd services/audit-service && go mod download

COPY shared ./shared
COPY services/audit-service ./services/audit-service

RUN cd services/audit-service && CGO_ENABLED=0 GOOS=linux go build -o /out/audit-service ./cmd/server

FROM gcr.io/distroless/base-debian12:nonroot
WORKDIR /
COPY --from=build /out/audit-service /audit-service
EXPOSE 8008
USER nonroot:nonroot
ENTRYPOINT ["/audit-service"]

