image_tag :=1.0.0-f
loggen_image_name := quay.io/ashish_amarnath/log-gen
logmon_image_name := quay.io/ashish_amarnath/log-mon
.PHONY: build/loggen loggen_image

buildDirloggen:
	mkdir -p buildloggen

buildDirlogmon:
	mkdir -p buildlogmon

build/loggen: loggen/*.go | buildDirloggen
	docker run -it \
		-v $(PWD):/go/src/github.com/ashish-amarnath/log-slo \
		-v $(PWD)/buildloggen:/go/bin \
		golang:1.7.4 \
			go build -v -o /go/bin/loggen \
			github.com/ashish-amarnath/log-slo/loggen

build/logmon: logmon/*.go | buildDirlogmon
	docker run -it \
		-v $(PWD):/go/src/github.com/ashish-amarnath/log-slo \
		-v $(PWD)/buildlogmon:/go/bin \
		golang:1.7.4 \
			go build -v -o /go/bin/logmon \
			github.com/ashish-amarnath/log-slo/logmon

build/loggenDockerfile: buildDirloggen
	cp loggenDockerfile buildloggen/Dockerfile

build/logmonDockerfile: buildDirlogmon
	cp logmonDockerfile buildlogmon/Dockerfile

loggen_image: build/loggen build/loggenDockerfile
	docker build -t $(loggen_image_name):$(image_tag) buildloggen

logmon_image: build/logmon build/logmonDockerfile
	docker build -t $(logmon_image_name):$(image_tag) buildlogmon

push_loggen_image: loggen_image
	docker push $(loggen_image_name):$(image_tag)

push_logmon_image: logmon_image
	docker push $(logmon_image_name):$(image_tag)

push_images: push_loggen_image push_logmon_image

all: logmon_image loggen_image

clean_loggen:
	rm -rf buildloggen/

clean_logmon:
	rm -rf buildlogmon/

clean: clean_loggen clean_logmon
