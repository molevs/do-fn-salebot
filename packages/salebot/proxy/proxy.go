package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"
)

type Event struct {
	// Init https://core.telegram.org/bots/webapps#webappinitdata
	Init    string      `json:"init"`
	GroupId interface{} `json:"group_id"`
	Message interface{} `json:"message"`
}

type Response struct {
	Body       ResponseBody      `json:"body"`
	StatusCode int               `json:"statusCode"`
	Headers    map[string]string `json:"headers"`
}

type ResponseBody struct {
	Status  string      `json:"status"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

const (
	TelegramBotToken            string = "TELEGRAM_BOT_TOKEN"
	ConstructorUrl              string = "CONSTRUCTOR_URL"
	ResponseStatusOk            string = "ok"
	ResponseStatusError         string = "error"
	ResponseStatusNotOk         string = "not_ok"
	ResponseMessageAccessDenied string = "access denied"
)

type Config struct {
	telegramBotToken string
	constructorUrl   string
}

type Verificator struct{}

type Responser struct {
	logger ResponseLogger
}

type HttpClientDoIt interface {
	Do(req *http.Request) (*http.Response, error)
}

type ContextValue interface {
	Value(key any) any
}

type ResponseLogger interface {
	Log(message []string)
}

type Sender struct {
	client HttpClientDoIt
}

type Logger struct {
	out       io.Writer
	requestId string
}

// WebAppUser https://core.telegram.org/bots/webapps#webappuser
type WebAppUser struct {
	Id int `json:"id"`
}

type ConnectionRefusedError struct {
	err error
}

type StatusCodeNotOkError struct {
	statusCode int
	body       []byte
}

func Main(ctx context.Context, event Event) Response {
	var responser Responser
	var verificator Verificator
	var sender Sender

	conf := newConfig()
	user := WebAppUser{}
	responser = Responser{logger: newLogger(ctx)}
	sender = Sender{client: &http.Client{}}

	values, err := url.ParseQuery(event.Init)
	if err != nil {
		return responser.BadRequest400(err.Error())
	}

	err = json.Unmarshal([]byte(values.Get("user")), &user)
	if err != nil {
		return responser.BadRequest400(err.Error())
	}

	err = verificator.verify(values, conf.telegramBotToken)
	if err != nil {
		return responser.Forbidden403(err.Error())
	}

	resp, err := sender.send(user, event, conf.constructorUrl)
	if err != nil {
		return responser.convertErrorToResponse(err)
	}

	return responser.Ok200(string(resp))
}

func newConfig() Config {
	return Config{
		telegramBotToken: Config{}.getEnvOrPanic(TelegramBotToken),
		constructorUrl:   Config{}.getUrlOrPanic(Config{}.getEnvOrPanic(ConstructorUrl)),
	}
}

func (c Config) getEnvOrPanic(key string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}

	panic(fmt.Sprintf("Required environment variable not set: `%s`", key))
}

func (c Config) getUrlOrPanic(str string) string {
	u, err := url.ParseRequestURI(str)
	if err != nil {
		panic(err)
	}

	if u.Scheme == "" || u.Host == "" {
		panic(fmt.Sprintf("Url should be like https://example.domain. Your url: `%s`", str))
	}

	return str
}

func newLogger(ctx ContextValue) Logger {
	requestId, ok := ctx.Value("request_id").(string)

	if !ok {
		panic(`The value "request_id" was not found in the context or it is not a string`)
	}

	return Logger{
		out:       os.Stdout,
		requestId: requestId,
	}
}

func (s Sender) send(user WebAppUser, event Event, constructorBotUrl string) ([]byte, error) {
	body, err := s.newBody(user, event)
	if err != nil {
		return nil, err
	}

	req, err := s.newRequest(body, constructorBotUrl)
	if err != nil {
		return nil, err
	}

	answer, err := s.do(req)
	if err != nil {
		return nil, err
	}

	return answer, nil
}

func (s Sender) newBody(user WebAppUser, event Event) ([]byte, error) {
	body, err := json.Marshal(map[string]interface{}{
		"user_id":  user.Id,
		"message":  event.Message,
		"group_id": event.GroupId,
	})

	if err != nil {
		return nil, err
	}

	return body, nil
}

func (s Sender) newRequest(body []byte, u string) (*http.Request, error) {
	req, err := http.NewRequest(http.MethodPost, u, bytes.NewBuffer(body))

	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json; charset=UTF-8")

	return req, nil
}

func (e ConnectionRefusedError) Error() string {
	return e.err.Error()
}

func (e StatusCodeNotOkError) Error() string {
	return string(e.body)
}

func (s Sender) do(req *http.Request) ([]byte, error) {
	resp, err := s.client.Do(req)

	if err != nil {
		return nil, ConnectionRefusedError{err: err}
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, StatusCodeNotOkError{
			statusCode: resp.StatusCode,
			body:       body,
		}
	}

	return body, nil
}

func (v Verificator) verify(values url.Values, token string) error {
	str := v.getCheckString(values)
	secret := v.getHashSum([]byte(token), []byte("WebAppData"))
	hash := v.getHashSum([]byte(str), secret)
	expectedHash := hex.EncodeToString(hash)

	if values.Get("hash") != expectedHash {
		return fmt.Errorf("hash sum doesn't match; expected hash: %s", expectedHash)
	}

	return nil
}

func (v Verificator) getCheckString(values url.Values) string {
	data := make([]string, 0, len(values))

	for key, value := range values {
		if key == "hash" {
			continue
		}

		data = append(data, fmt.Sprintf("%s=%s", key, value[0]))
	}

	sort.Strings(data)

	return strings.Join(data, "\n")
}

func (v Verificator) getHashSum(data, key []byte) []byte {
	hash := hmac.New(sha256.New, key)
	hash.Write(data)

	return hash.Sum(nil)
}

func (r Responser) convertErrorToResponse(err error) Response {
	var connectionRefusedError ConnectionRefusedError
	var statusCodeIsNotOkError StatusCodeNotOkError

	switch {
	case errors.As(err, &connectionRefusedError):
		return r.BadGateway502(err)
	case errors.As(err, &statusCodeIsNotOkError):
		return r.GatewaySayNotOk(statusCodeIsNotOkError)
	default:
		return r.InternalServerError500(err)
	}
}

func (r Responser) Ok200(body interface{}) Response {
	r.logger.Log([]string{"200 ok"})

	return r.response(
		ResponseBody{
			Status: ResponseStatusOk,
			Data:   body,
		},
		http.StatusOK,
	)
}

func (r Responser) BadRequest400(message string) Response {
	r.logger.Log([]string{"400 bad request", message})

	return r.response(
		ResponseBody{
			Status:  ResponseStatusError,
			Message: message,
		},
		http.StatusBadRequest,
	)
}

func (r Responser) Forbidden403(reason string) Response {
	r.logger.Log([]string{"403 Forbidden", reason})

	return r.response(
		ResponseBody{
			Status:  ResponseStatusError,
			Message: ResponseMessageAccessDenied,
		},
		http.StatusForbidden,
	)
}

func (r Responser) BadGateway502(err error) Response {
	r.logger.Log([]string{err.Error()})

	return r.response(
		ResponseBody{
			Status: ResponseStatusError,
		},
		http.StatusBadGateway,
	)
}

func (r Responser) GatewaySayNotOk(notOkError StatusCodeNotOkError) Response {
	r.logger.Log([]string{fmt.Sprintf("%d Gateway say not ok", notOkError.statusCode)})

	return r.response(
		ResponseBody{
			Status: ResponseStatusNotOk,
			Data:   string(notOkError.body),
		},
		notOkError.statusCode,
	)
}

func (r Responser) InternalServerError500(err error) Response {
	r.logger.Log([]string{err.Error()})

	return r.response(
		ResponseBody{
			Status: ResponseStatusError,
		},
		http.StatusInternalServerError,
	)
}

func (r Responser) response(body ResponseBody, status int) Response {
	return Response{
		Body:       body,
		StatusCode: status,
		Headers: map[string]string{
			"Content-type": "application/json",
		},
	}
}

func (l Logger) Log(message []string) {
	_, _ = fmt.Fprintf(
		l.out,
		"> request id: %s; time: %s; message: %s;\n",
		l.requestId,
		time.Now().UTC().Format(time.RFC3339),
		strings.Join(message, "; "),
	)
}
