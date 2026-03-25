build:
	mkdir -p bin
	go build -o bin/rbd-manager .

clean:
	rm -rf bin
