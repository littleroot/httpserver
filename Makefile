.PHONY: build
build:
	go build

.PHONY: scp
scp: build
	scp httpserver conf.toml nishanthshanmugham@tortoise:~/run/httpserver/

