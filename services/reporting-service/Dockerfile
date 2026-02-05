FROM golang:1.22-alpine AS build
WORKDIR /src

COPY go.work ./
COPY shared/go.mod shared/go.sum ./shared/
COPY services/reporting-service/go.mod services/reporting-service/go.sum ./services/reporting-service/

RUN cd services/reporting-service && go mod download

COPY shared ./shared
COPY services/reporting-service ./services/reporting-service

RUN cd services/reporting-service && CGO_ENABLED=0 GOOS=linux go build -o /out/reporting-service ./cmd/server

FROM gcr.io/distroless/base-debian12:nonroot
WORKDIR /
COPY --from=build /out/reporting-service /reporting-service
EXPOSE 8006
USER nonroot:nonroot
ENTRYPOINT ["/reporting-service"]

