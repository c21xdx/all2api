package tabbit

import (
	"bufio"
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lhpqaq/all2api/internal/config"
	"github.com/lhpqaq/all2api/internal/core"
	"github.com/lhpqaq/all2api/internal/upstream"
)

// modelMap maps standard names to Tabbit names
var modelMap = map[string]string{
	"best":              "最佳",
	"gpt-5.2-chat":      "GPT-5.2-Chat",
	"gpt-5.1-chat":      "GPT-5.1-Chat",
	"gemini-3.1-pro":    "Gemini-3.1-Pro",
	"gemini-3-flash":    "Gemini-3-Flash",
	"gemini-2.5-flash":  "Gemini-2.5-Flash",
	"claude-sonnet-4.6": "Claude-Sonnet-4.6",
	"claude-3.5-sonnet": "Claude-Sonnet-4.6",
	"claude-haiku-4.5":  "Claude-Haiku-4.5",
	"glm-5":             "GLM-5",
	"deepseek-v3.2":     "DeepSeek-V3.2",
	"minimax-m2.5":      "MiniMax-M2.5",
	"kimi-k2.5":         "Kimi-K2.5",
	"qwen3.5-plus":      "Qwen3.5-Plus",
	"doubao-seed-1.8":   "Doubao-Seed-1.8",
}

var tabbitBaseURL = "https://web.tabbitbrowser.com"

func generateUniqueUUID() string {
	markerPos := 5
	defaultBrowserMarker := "1"
	timestampPositions := []int{2, 7, 11, 14, 18, 21, 25, 28}
	hexChars := "0123456789abcdef"

	nowStr := fmt.Sprintf("%08x", time.Now().Unix())
	if len(nowStr) > 8 {
		nowStr = nowStr[len(nowStr)-8:]
	}
	tsMap := make(map[int]byte)
	for i, pos := range timestampPositions {
		tsMap[pos] = nowStr[i]
	}

	var o strings.Builder
	for a := 0; a < 32; a++ {
		if a == markerPos {
			o.WriteString(defaultBrowserMarker)
		} else if ch, ok := tsMap[a]; ok {
			o.WriteByte(ch)
		} else {
			o.WriteByte(hexChars[rand.Intn(len(hexChars))])
		}
	}
	s := o.String()
	return fmt.Sprintf("%s-%s-%s-%s-%s", s[:8], s[8:12], s[12:16], s[16:20], s[20:32])
}

type tabbitUpstream struct {
	client   *http.Client
	timeout  time.Duration
	jwtToken string
	nextAuth string
	deviceID string
	userID   string
}

func extractUserID(token string) string {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return uuid.New().String()
	}
	decoded, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return uuid.New().String()
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(decoded, &payload); err != nil {
		return uuid.New().String()
	}
	if id, ok := payload["id"].(string); ok {
		return id
	}
	if sub, ok := payload["sub"].(string); ok {
		return sub
	}
	return uuid.New().String()
}

func New(name string, cfg config.UpstreamConf) (upstream.Upstream, upstream.Capabilities, error) {
	// Parse token which is | separated
	tokenStr := cfg.Auth.Token
	parts := strings.Split(tokenStr, "|")

	var jwtToken, nextAuth, deviceID string
	if len(parts) > 0 {
		jwtToken = strings.TrimSpace(parts[0])
	}
	if len(parts) > 1 {
		nextAuth = strings.TrimSpace(parts[1])
	}
	if len(parts) > 2 {
		deviceID = strings.TrimSpace(parts[2])
	} else {
		deviceID = uuid.New().String()
	}

	uid := extractUserID(jwtToken)

	tr := &http.Transport{Proxy: http.ProxyFromEnvironment}
	if cfg.Proxy != "" {
		if pu, err := url.Parse(cfg.Proxy); err == nil {
			tr.Proxy = http.ProxyURL(pu)
		}
	}

	tu := &tabbitUpstream{
		client:   &http.Client{Transport: tr},
		timeout:  cfg.Timeout.Duration,
		jwtToken: jwtToken,
		nextAuth: nextAuth,
		deviceID: deviceID,
		userID:   uid,
	}

	cap := upstream.Capabilities{
		NativeToolCalls: false,
		SupportThinking: false,
	}
	return tu, cap, nil
}

// Generate headers
func (u *tabbitUpstream) getHeaders(payloadStr string) map[string]string {
	traceIDHex := strings.ReplaceAll(uuid.New().String(), "-", "")
	spanIDHex := traceIDHex[:16]

	timestampMs := fmt.Sprintf("%d", time.Now().UnixNano()/1e6)
	signatureUUID := uuid.New().String()

	payloadHash := sha256.Sum256([]byte(payloadStr))
	payloadHashHex := hex.EncodeToString(payloadHash[:])

	messageToSign := fmt.Sprintf("%s.%s.%s", timestampMs, signatureUUID, payloadHashHex)

	mac := hmac.New(sha256.New, []byte(""))
	mac.Write([]byte(messageToSign))
	nonceSignature := hex.EncodeToString(mac.Sum(nil))

	headers := map[string]string{
		"accept":                          "text/event-stream, application/json",
		"accept-language":                 "zh-CN,zh;q=0.9",
		"baggage":                         fmt.Sprintf("sentry-environment=production,sentry-release=0a8bc01,sentry-public_key=4a5c74385c227d3ba012317b37a9e6c5,sentry-trace_id=%s,sentry-transaction=%%2Fchat%%2F%%3Aid,sentry-sampled=false,sentry-sample_rand=0.08096,sentry-sample_rate=0", traceIDHex),
		"cache-control":                   "no-cache",
		"content-type":                    "application/json",
		"sec-ch-ua":                       `"Not:A-Brand";v="99", "Tabbit";v="145", "Chromium";v="145"`,
		"sec-ch-ua-mobile":                "?0",
		"sec-ch-ua-platform":              `"macOS"`,
		"sec-fetch-dest":                  "empty",
		"sec-fetch-mode":                  "cors",
		"sec-fetch-site":                  "same-origin",
		"sentry-trace":                    fmt.Sprintf("%s-%s-0", traceIDHex, spanIDHex),
		"origin":                          tabbitBaseURL,
		"x-chrome-id-consistency-request": fmt.Sprintf("version=1,client_id=%s,device_id=%s,sync_account_id=%s,signin_mode=all_accounts,signout_mode=show_confirmation", "clientID", u.deviceID, u.userID),
		"referer":                         tabbitBaseURL + "/newtab",
		"trace-id":                        uuid.New().String(),
		"unique-uuid":                     generateUniqueUUID(),
		"priority":                        "u=1, i",
		"User-Agent":                      "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/145.0.0.0 (stable) Safari/537.36",
		"x-timestamp":                     timestampMs,
		"x-signature":                     signatureUUID,
		"x-nonce":                         nonceSignature,
	}
	return headers
}

func (u *tabbitUpstream) getCookieStr() string {
	var cookies []string
	if u.jwtToken != "" {
		cookies = append(cookies, "token="+u.jwtToken)
	}
	if u.userID != "" {
		cookies = append(cookies, "user_id="+u.userID)
		cookies = append(cookies, "SAPISID="+u.userID)
	}
	cookies = append(cookies, "managed=tab_browser")
	cookies = append(cookies, "NEXT_LOCALE=zh")
	cookies = append(cookies, "expires_in=604800")
	cookies = append(cookies, `g_state={"i_l":0,"i_ll":1773679217003,"i_e":{"enable_itp_optimization":1}}`)
	if u.nextAuth != "" {
		cookies = append(cookies, "next-auth.session-token="+u.nextAuth)
	}
	return strings.Join(cookies, "; ")
}

// compactJSON precisely serializes a map to JSON string with `ensure_ascii=False, separators=(",",":")` semantics.
func compactJSON(v interface{}) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	// Do not return error, handle gracefully
	_ = enc.Encode(v)
	// Output has trailing newline, remove it
	s := buf.String()
	if len(s) > 0 && s[len(s)-1] == '\n' {
		s = s[:len(s)-1]
	}

	// Ensure keys are correctly formatted (though json package handles it, spaces after comma/colon are stripped by default encoding map)
	// However, json.Marshal output no spaces by default, so we're good.
	return s
}

func buildContent(messages []core.Message) string {
	var sb strings.Builder
	for _, m := range messages {
		text := m.Content
		if m.Role == "user" {
			sb.WriteString(text + "\n")
		} else if m.Role == "assistant" {
			sb.WriteString(text + "\n")
		} else if m.Role == "system" {
			sb.WriteString(text + "\n")
		}
	}
	return strings.TrimSpace(sb.String())
}

func (u *tabbitUpstream) Do(ctx context.Context, req core.CoreRequest) (core.CoreResult, error) {
	var cancel context.CancelFunc
	if u.timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, u.timeout)
	}

	model, ok := modelMap[strings.ToLower(req.Model)]
	if !ok {
		model = "最佳"
	}

	content := buildContent(req.Messages)
	if req.System != "" {
		content = req.System + "\n\n" + content
	}

	md5Hash := md5.Sum([]byte(""))
	md5Hex := hex.EncodeToString(md5Hash[:])

	payload := map[string]interface{}{
		"message_id":     nil,
		"content":        content,
		"selected_model": model,
		"agent_mode":     false,
		"metadatas": map[string]interface{}{
			"html_content": fmt.Sprintf("<p>%s</p>", strings.ReplaceAll(content, "\n", "<br>")),
		},
		"references": []interface{}{},
		"entity": map[string]interface{}{
			"key": md5Hex,
			"extras": map[string]interface{}{
				"type": "tab",
				"url":  "",
			},
		},
	}

	payloadStr := compactJSON(payload)
	fmt.Printf("TABBIT REQUEST PAYLOAD: %s\n", payloadStr)
	headers := u.getHeaders(payloadStr)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", tabbitBaseURL+"/chat/send", strings.NewReader(payloadStr))
	if err != nil {
		return core.CoreResult{}, err
	}

	for k, v := range headers {
		httpReq.Header.Set(k, v)
	}
	httpReq.Header.Set("Cookie", u.getCookieStr())

	resp, err := u.client.Do(httpReq)
	if err != nil {
		return core.CoreResult{}, fmt.Errorf("do request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return core.CoreResult{}, fmt.Errorf("tabbit upstream HTTP %d: %s", resp.StatusCode, string(b))
	}

	// Tabbit responds using SSE. We always parse the SSE stream and either:
	//  - stream deltas to req.StreamChannel (when provided), and/or
	//  - accumulate full text and return it.
	defer resp.Body.Close()
	if cancel != nil {
		defer cancel()
	}

	var fullText strings.Builder

	scanner := bufio.NewScanner(resp.Body)
	buf := make([]byte, 0, 64*1024)
	// Tool results (e.g. web_search summaries) can be large; allow bigger lines.
	scanner.Buffer(buf, 8*1024*1024)

	currentEvent := ""
	for scanner.Scan() {
		line := scanner.Text()
		fmt.Println("RAW LINE:", line)

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "event:") {
			currentEvent = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}

		dataStr := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if dataStr == "[DONE]" || dataStr == "" {
			continue
		}

		var dataObj map[string]interface{}
		if err := json.Unmarshal([]byte(dataStr), &dataObj); err != nil {
			continue
		}

		switch currentEvent {
		case "message_chunk":
			if cnt, ok := dataObj["content"].(string); ok {
				if cnt != "" {
					fullText.WriteString(cnt)
					if req.StreamChannel != nil {
						req.StreamChannel <- core.StreamEvent{TextDelta: cnt}
					}
				}
			}
		case "error":
			if msg, ok := dataObj["message"].(string); ok && strings.TrimSpace(msg) != "" {
				if req.StreamChannel != nil {
					req.StreamChannel <- core.StreamEvent{Error: fmt.Errorf("tabbit error: %s", msg)}
					close(req.StreamChannel)
				}
				return core.CoreResult{Text: fullText.String()}, fmt.Errorf("tabbit error: %s", msg)
			}
		default:
			// ignore other events (ready/usage/tool_start/tool_finish/...)
		}
	}

	fmt.Printf("Scanner ended stream, err: %v\\n", scanner.Err())
	if err := scanner.Err(); err != nil {
		// Downstream disconnects often cancel the context; don't treat that as upstream failure.
		if errors.Is(err, context.Canceled) {
			if req.StreamChannel != nil {
				close(req.StreamChannel)
			}
			return core.CoreResult{Text: fullText.String()}, nil
		}
		if req.StreamChannel != nil {
			req.StreamChannel <- core.StreamEvent{Error: err}
			close(req.StreamChannel)
		}
		return core.CoreResult{Text: fullText.String()}, err
	}

	if req.StreamChannel != nil {
		req.StreamChannel <- core.StreamEvent{Done: true}
		close(req.StreamChannel)
	}

	return core.CoreResult{Text: fullText.String()}, nil
}

// ListModels implements upstream.ModelLister
func (u *tabbitUpstream) ListModels(ctx context.Context) ([]string, error) {
	models := make([]string, 0, len(modelMap))
	for k := range modelMap {
		models = append(models, k)
	}
	return models, nil
}
