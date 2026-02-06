FROM golang:1.22-alpine AS build
WORKDIR /src

COPY shared/go.mod shared/go.sum ./shared/
COPY services/budget-service/go.mod services/budget-service/go.sum ./services/budget-service/

RUN cd services/budget-service && go mod download

COPY shared ./shared
COPY services/budget-service ./services/budget-service

RUN cd services/budget-service && CGO_ENABLED=0 GOOS=linux go build -o /out/budget-service ./cmd/server

FROM gcr.io/distroless/base-debian12:nonroot
WORKDIR /
COPY --from=build /out/budget-service /budget-service
EXPOSE 8007
USER nonroot:nonroot
ENTRYPOINT ["/budget-service"]
