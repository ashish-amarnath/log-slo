package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"regexp"
	// "fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Shopify/sarama"
	"github.com/golang/glog"
)

var (
	// FLAGS
	helpFlag       *bool
	brokersStr     *string
	topic          *string
	logID          *string
	consumerOffset *string
	logTarget      *int64
)

type lmsg struct {
	Timestamp string `json:"@timestamp"`
	ClusterID string `json:"cluster_id"`
	InputType string `json:"input_type"`
	Message   string `json:"message"`
	Offset    int64  `json:"offset"`
	Source    string `json:"source"`
	Type      string `json:"type"`
}

type commit struct {
	Offset int64
	POM    *sarama.PartitionOffsetManager
}

type kafkamessage struct {
	Event *sarama.ConsumerMessage
	POM   *sarama.PartitionOffsetManager
}

func main() {
	// DEFAULTS
	dBrokers := os.Getenv("BROKERS")
	if dBrokers == "" {
		dBrokers = "127.0.0.1:9092"
	}

	dTopic := os.Getenv("TOPIC")
	if dTopic == "" {
		dTopic = "default"
	}

	dLID := os.Getenv("LOGID")
	if dLID == "" {
		dLID = "LogGenerator"
	}

	dLogTarget, err := strconv.ParseInt(os.Getenv("LOG_TARGET"), 10, 64)
	if err != nil || dLogTarget == 0 {
		dLogTarget = 900000
	}

	group := "loggenmonclient"

	// FLAGS
	helpFlag = flag.Bool("help", false, "")
	brokersStr = flag.String("brokers", dBrokers, "Kafka brokers, comma seperated (ex: '127.0.0.1:9092,localhost:9092'). Env: BROKERS or 127.0.0.1:9092")
	topic = flag.String("topic", dTopic, "Kafka topic to consume. Env: TOPIC or 'default'")
	logID = flag.String("logid", dLID, "String used to identify this particular log. Env: LOGID or LogGenerator")
	consumerOffset = flag.String("consumer-offset", "default", "beginning|newest|resume|default Default: default")
	logTarget = flag.Int64("logtarget", dLogTarget, "expected number of log line")

	flag.Parse()

	if *helpFlag {
		flag.PrintDefaults()
		os.Exit(0)
	}

	glog.V(1).Infof("Using flags:\n	brokers: %s\n topic: %s\n logId: %s\n consumerOffset: %s\n logTarget:%d\n",
		*brokersStr, *topic, *logID, *consumerOffset, *logTarget)

	// init (custom) config, enable errors and notifications
	config := sarama.NewConfig()
	config.ClientID = group
	config.Consumer.Return.Errors = true
	config.Consumer.Offsets.CommitInterval = time.Duration(1) * time.Second

	// init consumer
	brokers := strings.Split(*brokersStr, ",")

	glog.V(1).Infof("Created new client with %s\n", strings.Join(brokers, ","))
	client, err := sarama.NewClient(brokers, config)
	if err != nil {
		panic(err)
	}

	consumer, err := sarama.NewConsumer(brokers, config)
	if err != nil {
		panic(err)
	}

	defer func() {
		if err := consumer.Close(); err != nil {
			panic(err)
		}
	}()

	// trap SIGINT to trigger a shutdown.
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)

	partitionList, err := consumer.Partitions(*topic)
	if err != nil {
		panic(err)
	}
	pListStr := ""
	for _, p := range partitionList {
		pListStr += fmt.Sprintf("%d,", p)
	}

	glog.V(1).Infof("partition list:%s", pListStr)

	offsetManager, err := sarama.NewOffsetManagerFromClient(group, client)
	if err != nil {
		panic(err)
	}

	var initialOffset int64
	var relativeOffset int
	switch *consumerOffset {
	case "beginning":
		initialOffset = sarama.OffsetOldest
	case "newest":
		initialOffset = sarama.OffsetNewest
	case "resume":
		initialOffset = sarama.OffsetOldest
	case "default":
		relativeOffset, err := strconv.ParseInt(*consumerOffset, 10, 64)
		initialOffset = sarama.OffsetOldest
		if relativeOffset < 0 {
			initialOffset = sarama.OffsetNewest
		}

		if err != nil {
			glog.Fatalf("Invalid value for --consumer-offset, %v", err)
			flag.PrintDefaults()
			os.Exit(1)
		}
	}

	var (
		messages = make(chan kafkamessage, 512)
		closing  = make(chan struct{})
		commits  = make(chan commit)
		wg       sync.WaitGroup
	)

	go func() {
		signals := make(chan os.Signal, 1)
		signal.Notify(signals, os.Kill, os.Interrupt)
		<-signals
		glog.V(1).Infoln("Consumer shutting down...")
		close(closing)
	}()

	for _, partition := range partitionList {
		partOffsetManager, err := offsetManager.ManagePartition(*topic, partition)
		if err != nil {
			glog.Warningf("Manage partition (%s, %d) failed: %s", *topic, partition, err)
		}
		partitionOffset := initialOffset + int64(relativeOffset)
		if *consumerOffset == "resume" {
			partitionOffset, _ = partOffsetManager.NextOffset()
			if partitionOffset < 0 {
				partitionOffset = -2
			}
		}
		glog.V(1).Infof("Starting consumer of partition: %d at offset: %d.\n", partition, partitionOffset)
		pc, err := consumer.ConsumePartition(*topic, partition, partitionOffset)
		if err != nil {
			glog.Fatalf("Failed to start consumer for partition %d, %s.\n", partition, err)

		}

		go func(pc sarama.PartitionConsumer, pom sarama.PartitionOffsetManager) {
			<-closing
			pom.Close()
			pc.AsyncClose()
		}(pc, partOffsetManager)

		wg.Add(1)

		go func(pc sarama.PartitionConsumer, pom sarama.PartitionOffsetManager) {
			defer wg.Done()
			for message := range pc.Messages() {
				msg := kafkamessage{Event: message, POM: &pom}
				messages <- msg
			}
		}(pc, partOffsetManager)
	}

	go func() {
		glog.V(1).Infof("starting message reader")
		//re, err := regexp.Compile("^I.*] (.*) (\\d+)")
		//re, err := regexp.Compile("\"I.*] (.*):(\\d+)")
		//if err != nil {
		//	glog.Fatalf("Couldn't compile regex: %s\n", err)
		//}
		regexStr := fmt.Sprintf("\"I.*] (%s):(\\d+)", *logID)
		re := regexp.MustCompile(regexStr)

		for m := range messages {
			var decoded lmsg
			cmsg := *m.Event
			err := json.Unmarshal(cmsg.Value, &decoded)
			if err != nil {
				glog.Fatalf("JSON decode error: %s with %s.\n", err, cmsg.Value)
			}
			if !re.MatchString(decoded.Message) {
				glog.V(9).Infof("NoMatched %s\n", regexStr)
				continue
			}
			glog.V(1).Infof("%s", decoded)
		}
	}()

	go func() {
		for commit := range commits {
			glog.V(5).Infof("Commiting %d.\n", commit.Offset)
			pom := *commit.POM
			pom.MarkOffset(commit.Offset, "")
		}
	}()

	glog.V(1).Infof("Main thread waiting for wait group threads")
	wg.Wait()
}
