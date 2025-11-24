package middleware

import (
	"encoding/json"
	"time"
	
	"github.com/SkynetNext/unified-access-gateway/pkg/xlog"
)

// AccessLog defines the structure of access logs
type AccessLog struct {
	Timestamp   time.Time `json:"ts"`
	ClientIP    string    `json:"client_ip"`
	Protocol    string    `json:"protocol"` // HTTP, TCP
	Method      string    `json:"method,omitempty"` // HTTP only
	Path        string    `json:"path,omitempty"`   // HTTP only
	DurationMs  int64     `json:"duration_ms"`
	Status      int       `json:"status"`
	BytesIn     int64     `json:"bytes_in"`
	BytesOut    int64     `json:"bytes_out"`
}

type Logger struct {
	logChan chan *AccessLog
}

var Instance *Logger

func InitLogger(bufferSize int) {
	Instance = &Logger{
		logChan: make(chan *AccessLog, bufferSize),
	}
	go Instance.startConsumer()
}

func (l *Logger) Log(entry *AccessLog) {
	select {
	case l.logChan <- entry:
	default:
		// Buffer full, drop log to prevent blocking main flow
		xlog.Warnf("Access log buffer full, dropping log")
	}
}

func (l *Logger) startConsumer() {
	// Simulate batch sending to Kafka
	// In production, use sarama.AsyncProducer
	batch := make([]*AccessLog, 0, 100)
	ticker := time.NewTicker(1 * time.Second)
	
	for {
		select {
		case entry := <-l.logChan:
			batch = append(batch, entry)
			if len(batch) >= 100 {
				l.flushToKafka(batch)
				batch = batch[:0]
			}
		case <-ticker.C:
			if len(batch) > 0 {
				l.flushToKafka(batch)
				batch = batch[:0]
			}
		}
	}
}

func (l *Logger) flushToKafka(logs []*AccessLog) {
	// Mock: Print to console, actually produce to Kafka Topic
	xlog.Infof("Flushing %d access logs to Kafka...", len(logs))
	for _, log := range logs {
		data, _ := json.Marshal(log)
		// In real scenario: producer.Input() <- &sarama.ProducerMessage{...}
		// Print only the first log for demo
		xlog.Debugf("Kafka Log Payload: %s", string(data))
		break 
	}
}
