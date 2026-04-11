package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

const redactedValue = "[REDACTED]"

type RequestLogger struct {
	logger zerolog.Logger
}

type bodyLogWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func NewRequestLogger(logger zerolog.Logger) *RequestLogger {
	return &RequestLogger{
		logger: logger.With().Str("component", "middleware.request_logger").Logger(),
	}
}

func (m *RequestLogger) Handler() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		bodyBytes := m.readRequestBody(c)

		m.logger.Info().
			Str("method", c.Request.Method).
			Str("path", c.Request.URL.Path).
			Str("query", c.Request.URL.RawQuery).
			Str("request_body", sanitizeBody(bodyBytes)).
			Str("client_ip", c.ClientIP()).
			Str("user_agent", c.Request.UserAgent()).
			Msg("request received")

		blw := &bodyLogWriter{
			ResponseWriter: c.Writer,
			body:           bytes.NewBufferString(""),
		}
		c.Writer = blw

		c.Next()

		m.logger.Info().
			Int("status", c.Writer.Status()).
			Str("response_body", sanitizeBody(blw.body.Bytes())).
			Int64("latency_ms", time.Since(start).Milliseconds()).
			Msg("response sent")
	}
}

func (m *RequestLogger) readRequestBody(c *gin.Context) []byte {
	if c.Request.Body == nil {
		return nil
	}

	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		m.logger.Error().Err(err).Msg("read request body failed")
		c.Request.Body = io.NopCloser(bytes.NewBuffer(nil))
		return nil
	}

	c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	return bodyBytes
}

func sanitizeBody(body []byte) string {
	if len(body) == 0 {
		return ""
	}

	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return ""
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(trimmed, &payload); err != nil {
		return string(trimmed)
	}

	if _, ok := payload["auth_key"]; ok {
		payload["auth_key"] = redactedValue
	}

	sanitized, err := json.Marshal(payload)
	if err != nil {
		return string(trimmed)
	}

	return string(sanitized)
}

func (w *bodyLogWriter) Write(b []byte) (int, error) {
	if _, err := w.body.Write(b); err != nil {
		return 0, err
	}

	return w.ResponseWriter.Write(b)
}

func (w *bodyLogWriter) WriteString(s string) (int, error) {
	if _, err := w.body.WriteString(s); err != nil {
		return 0, err
	}

	return w.ResponseWriter.WriteString(s)
}
