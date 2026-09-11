.PHONY: test race integration up monitoring down load

test:
	go test ./...

race:
	go test -race ./...

integration:
	go test -tags=integration ./tests/integration/...

up:
	docker compose up --build -d

monitoring:
	docker compose --profile observability up --build -d

down:
	docker compose down

load:
	k6 run tests/load/vote.js
