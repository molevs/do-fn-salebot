package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/molevs/do-fn-salebot/packages/salebot/proxy/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestConfig_getEnvOrPanic(t *testing.T) {
	type args struct {
		key string
	}
	successfulTests := []struct {
		name string
		args args
	}{
		{
			name: "exists env",
			args: args{
				key: "EXIST_ENV",
			},
		},
	}
	for _, tt := range successfulTests {
		t.Run(tt.name, func(t *testing.T) {
			_ = os.Setenv(tt.args.key, "exists")

			Config{}.getEnvOrPanic(tt.args.key)
		})
	}

	failTests := []struct {
		name     string
		args     args
		expected string
	}{
		{
			name: "not exists env",
			args: args{
				key: "NOT_EXIST_ENV",
			},
			expected: "Required environment variable not set: `NOT_EXIST_ENV`",
		},
	}
	for _, tt := range failTests {
		t.Run(tt.name, func(t *testing.T) {
			fn := func() { Config{}.getEnvOrPanic(tt.args.key) }
			assert.PanicsWithValue(t, tt.expected, fn)
		})
	}
}

func TestConfig_getUrlOrPanic(t *testing.T) {
	type args struct {
		str string
	}
	successfulTests := []struct {
		name     string
		args     args
		expected string
	}{
		{
			name: "correct url: schema://name.domain",
			args: args{
				str: "schema://name.domain",
			},
			expected: "schema://name.domain",
		},
		{
			name: "correct url: schema://sub-domain.name.domain/path?query=value#anchor",
			args: args{
				str: "schema://sub-domain.name.domain/path?query=value#anchor",
			},
			expected: "schema://sub-domain.name.domain/path?query=value#anchor",
		},
	}
	for _, tt := range successfulTests {
		t.Run(tt.name, func(t *testing.T) {
			actual := Config{}.getUrlOrPanic(tt.args.str)

			assert.Equal(t, tt.expected, actual)
		})
	}

	failTests := []struct {
		name     string
		args     args
		expected []string
	}{
		{
			name: "incorrect url: ",
			args: args{
				str: "",
			},
			expected: nil,
		},
		{
			name: "incorrect url: localhost:8080",
			args: args{
				str: "localhost:8080",
			},
			expected: []string{"Url should be like https://example.domain. Your url: `localhost:8080`"},
		},
	}
	for _, tt := range failTests {
		t.Run(tt.name, func(t *testing.T) {
			fn := func() { Config{}.getUrlOrPanic(tt.args.str) }

			if tt.expected != nil {
				assert.PanicsWithValue(t, tt.expected[0], fn)
			} else {
				assert.Panics(t, fn)
			}
		})
	}
}

func TestConnectionRefusedError_Error(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		message := "text text text"
		e := ConnectionRefusedError{
			err: errors.New(message),
		}

		assert.Equal(t, message, e.Error())
	})
}

func TestLogger_log(t *testing.T) {
	t.Run("logger", func(t *testing.T) {
		buffer := &bytes.Buffer{}

		l := Logger{
			out:       buffer,
			requestId: "123456",
		}

		l.Log([]string{"message 1", "message 2"})

		re := regexp.MustCompile(`\d\d\d\d-\d\d-\d\dT\d\d:\d\d:\d\dZ(\d\d:\d\d)*`)
		actual := re.ReplaceAll(buffer.Bytes(), []byte("__time__"))

		expected := fmt.Sprint("> request id: 123456; time: __time__; message: message 1; message 2;\n")

		assert.Equal(t, expected, string(actual))
	})
}

func TestResponser_BadGateway502(t *testing.T) {
	t.Run("bad gateway 502", func(t *testing.T) {
		err := errors.New("Houston, server has problems")

		logger := mocks.NewResponseLogger(t)
		logger.EXPECT().Log([]string{err.Error()})

		expected := Response{
			Body: ResponseBody{
				Status: ResponseStatusError,
			},
			StatusCode: http.StatusBadGateway,
			Headers: map[string]string{
				"Content-type": "application/json",
			},
		}

		assert.Equal(t, expected, Responser{logger: logger}.BadGateway502(err))
	})
}

func TestResponser_BadRequest400(t *testing.T) {
	t.Run("bad request 400", func(t *testing.T) {
		message := "Houston, client has problems"

		logger := mocks.NewResponseLogger(t)
		logger.EXPECT().Log([]string{"400 bad request", message})

		expected := Response{
			Body: ResponseBody{
				Status:  ResponseStatusError,
				Message: message,
			},
			StatusCode: 400,
			Headers: map[string]string{
				"Content-type": "application/json",
			},
		}

		assert.Equal(t, expected, Responser{logger: logger}.BadRequest400(message))
	})
}

func TestResponser_convertErrorToResponse(t *testing.T) {
	type args struct {
		err error
	}
	tests := []struct {
		name     string
		args     args
		expected Response
	}{
		{
			name: "ConnectionRefusedError",
			args: args{
				err: ConnectionRefusedError{err: errors.New("connection_refused_error")},
			},
			expected: Response{
				Body: ResponseBody{
					Status: ResponseStatusError,
				},
				StatusCode: http.StatusBadGateway,
				Headers: map[string]string{
					"Content-type": "application/json",
				},
			},
		},
		{
			name: "StatusCodeNotOkError",
			args: args{
				err: StatusCodeNotOkError{
					statusCode: 777,
					body:       []byte(`{"status_code_is_not_ok_error"}`),
				},
			},
			expected: Response{
				Body: ResponseBody{
					Status: ResponseStatusNotOk,
					Data:   `{"status_code_is_not_ok_error"}`,
				},
				StatusCode: 777,
				Headers: map[string]string{
					"Content-type": "application/json",
				},
			},
		},
		{
			name: "InternalServerError500",
			args: args{
				err: errors.New("internal server error"),
			},
			expected: Response{
				Body: ResponseBody{
					Status: ResponseStatusError,
				},
				StatusCode: http.StatusInternalServerError,
				Headers: map[string]string{
					"Content-type": "application/json",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := mocks.NewResponseLogger(t)
			logger.On("Log", mock.AnythingOfType("[]string")).Maybe()

			actual := Responser{logger: logger}.convertErrorToResponse(tt.args.err)

			assert.Equal(t, tt.expected, actual)
		})
	}
}

func TestResponser_Forbidden403(t *testing.T) {
	t.Run("Forbidden 403", func(t *testing.T) {
		messages := []string{"403 Forbidden", "additional information"}

		logger := mocks.NewResponseLogger(t)
		logger.On("Log", messages)

		expected := Response{
			Body: ResponseBody{
				Status:  ResponseStatusError,
				Message: ResponseMessageAccessDenied,
			},

			StatusCode: 403,
			Headers: map[string]string{
				"Content-type": "application/json",
			},
		}

		assert.Equal(t, expected, Responser{logger: logger}.Forbidden403(messages[1]))
	})
}

func TestResponser_Ok200(t *testing.T) {
	t.Run("ok 200", func(t *testing.T) {
		message := "200 ok"
		body := `{"all ok"}`

		logger := mocks.NewResponseLogger(t)
		logger.On("Log", []string{message})

		expected := Response{
			Body: ResponseBody{
				Status: ResponseStatusOk,
				Data:   body,
			},
			StatusCode: 200,
			Headers: map[string]string{
				"Content-type": "application/json",
			},
		}

		assert.Equal(t, expected, Responser{logger: logger}.Ok200(body))
	})
}

func TestResponser_GatewaySayNotOk(t *testing.T) {
	t.Run("gateway say not ok", func(t *testing.T) {
		logger := mocks.NewResponseLogger(t)
		logger.EXPECT().Log([]string{"777 Gateway say not ok"})

		body := `{"not ok"}`
		notOkError := StatusCodeNotOkError{
			statusCode: 777,
			body:       []byte(body),
		}

		expected := Response{
			Body: ResponseBody{
				Status: ResponseStatusNotOk,
				Data:   body,
			},
			StatusCode: notOkError.statusCode,
			Headers: map[string]string{
				"Content-type": "application/json",
			},
		}

		assert.Equal(t, expected, Responser{logger: logger}.GatewaySayNotOk(notOkError))
	})
}

func TestResponser_InternalServerError500(t *testing.T) {
	t.Run("ok 200", func(t *testing.T) {
		err := errors.New("Houston, server has problems")

		logger := mocks.NewResponseLogger(t)
		logger.On("Log", []string{err.Error()})

		expected := Response{
			Body: ResponseBody{
				Status: ResponseStatusError,
			},
			StatusCode: http.StatusInternalServerError,
			Headers: map[string]string{
				"Content-type": "application/json",
			},
		}

		assert.Equal(t, expected, Responser{logger: logger}.InternalServerError500(err))
	})
}

func TestResponser_response(t *testing.T) {
	t.Run("ok 200", func(t *testing.T) {
		logger := mocks.NewResponseLogger(t)

		body := ResponseBody{
			Status: ResponseStatusOk,
			Data:   `{"data":{...}}`,
		}

		expected := Response{
			Body:       body,
			StatusCode: 999,
			Headers: map[string]string{
				"Content-type": "application/json",
			},
		}

		assert.Equal(t, expected, Responser{logger: logger}.response(body, 999))
	})
}

func TestSender_do(t *testing.T) {
	type args struct {
		resp *http.Response
		err  error
	}
	type expected struct {
		body []byte
		err  error
	}
	tests := []struct {
		name     string
		args     args
		expected expected
	}{
		{
			name: "Successful",
			args: args{
				resp: &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`{"ping-pong"}`)),
				},
				err: nil,
			},
			expected: expected{
				body: []byte(`{"ping-pong"}`),
				err:  nil,
			},
		},
		{
			name: "fail Status code is not ok",
			args: args{
				resp: &http.Response{
					StatusCode: http.StatusBadRequest,
					Body:       io.NopCloser(strings.NewReader(`{"client error"}`)),
				},
				err: nil,
			},
			expected: expected{
				body: nil,
				err: StatusCodeNotOkError{
					statusCode: http.StatusBadRequest,
					body:       []byte(`{"client error"}`),
				},
			},
		},
		{
			name: "fail Connection Refused",
			args: args{
				resp: &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`{"ping-pong"}`)),
				},
				err: errors.New("connection refused"),
			},
			expected: expected{
				body: nil,
				err:  ConnectionRefusedError{err: errors.New("connection refused")},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := "https://localhost:1234/path"
			req, _ := http.NewRequest(http.MethodPost, u, nil)

			client := mocks.NewHttpClientDoIt(t)
			client.EXPECT().Do(req).Return(tt.args.resp, tt.args.err)
			sender := Sender{client: client}

			body, err := sender.do(req)

			if body != nil {
				assert.Equal(t, tt.expected.body, body)
				assert.Nil(t, err)
			} else {
				assert.Equal(t, tt.expected.err, err)
				assert.Nil(t, body)
			}
		})
	}
}

func TestSender_newBody(t *testing.T) {
	type args struct {
		user  WebAppUser
		event Event
	}
	successfulTests := []struct {
		name string
		args args
		want []byte
	}{
		{
			name: "correct body",
			args: args{
				user: WebAppUser{
					Id: 123456789,
				},
				event: Event{
					Init:    "",
					GroupId: "GroupId:123",
					Message: "event message",
				},
			},
			want: []byte(`{"group_id":"GroupId:123","message":"event message","user_id":123456789}`),
		},
	}
	for _, tt := range successfulTests {
		t.Run(tt.name, func(t *testing.T) {
			actual, _ := Sender{}.newBody(tt.args.user, tt.args.event)

			assert.Equal(t, tt.want, actual)
		})
	}
}

func TestSender_newRequest(t *testing.T) {
	t.Run("create request", func(t *testing.T) {
		body := "hello world!"
		domain := "schema://h0st.com/path1?query=value"

		req, _ := Sender{}.newRequest([]byte(body), domain)
		reqBody, _ := io.ReadAll(req.Body)

		assert.Equal(t, http.MethodPost, req.Method)
		assert.Equal(t, "application/json; charset=UTF-8", req.Header.Get("Content-Type"))
		assert.Equal(t, domain, req.URL.String())
		assert.Equal(t, body, string(reqBody))
	})
}

func TestSender_send(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		user := WebAppUser{Id: 12345}
		event := Event{
			GroupId: "group id",
			Message: "message",
		}
		constructorBotUrl := "https://localhost:1234/path"

		resp := &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"ping-pong"}`)),
		}

		client := mocks.NewHttpClientDoIt(t)
		client.EXPECT().Do(mock.AnythingOfType("*http.Request")).Return(resp, nil)
		sender := Sender{client: client}

		answer, _ := sender.send(user, event, constructorBotUrl)

		assert.Equal(t, []byte(`{"ping-pong"}`), answer)
	})
}

func TestStatusCodeIsNotOKError_Error(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		code := 111
		body := []byte("error :(")
		e := StatusCodeNotOkError{
			statusCode: code,
			body:       body,
		}

		assert.Equal(t, string(body), e.Error())
	})

}

func TestVerificator_getCheckString(t *testing.T) {
	type args struct {
		values url.Values
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "Unordered data",
			args: args{
				values: url.Values{
					"z":    []string{"z-text-1", "z-text-2"},
					"k":    []string{"k-text"},
					"hash": []string{"hash-text"},
					"a":    []string{"a-text"},
				},
			},
			want: "a=a-text\nk=k-text\nz=z-text-1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := Verificator{}
			if got := v.getCheckString(tt.args.values); got != tt.want {
				t.Errorf("getCheckString() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestVerificator_getHashSum(t *testing.T) {
	type args struct {
		data []byte
		key  []byte
	}
	tests := []struct {
		name     string
		args     args
		expected []byte
	}{
		{
			name: "correct hash sum",
			args: args{
				data: []byte("data"),
				key:  []byte("key"),
			},
			expected: []byte{0x50, 0x31, 0xfe, 0x3d, 0x98, 0x9c, 0x6d, 0x15, 0x37, 0xa0, 0x13, 0xfa, 0x6e, 0x73, 0x9d, 0xa2, 0x34, 0x63, 0xfd, 0xae, 0xc3, 0xb7, 0x1, 0x37, 0xd8, 0x28, 0xe3, 0x6a, 0xce, 0x22, 0x1b, 0xd0},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := Verificator{}.getHashSum(tt.args.data, tt.args.key)

			assert.Equal(t, tt.expected, actual)
		})
	}
}

func TestVerificator_verify(t *testing.T) {
	type args struct {
		values url.Values
		token  string
	}
	tests := []struct {
		name     string
		args     args
		expected error
	}{
		{
			name: "verify true",
			args: args{
				values: url.Values{
					"field": []string{"value"},
					"hash":  []string{"d678b8126526b5e47ec7da54a11416416238a1fa45d9890aeefffac8fb8c629e"},
				},
				token: "token",
			},
			expected: nil,
		},
		{
			name: "verify false",
			args: args{
				values: url.Values{
					"field1": []string{"value1"},
					"hash":   []string{"incorrect hash"},
				},
				token: "token",
			},
			expected: fmt.Errorf("hash sum doesn't match; expected hash: %s", "790d3e185970bd68f4c3cc81e7ae971a7b929e5bea7b21327ee9173652fa2853"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := Verificator{}.verify(tt.args.values, tt.args.token)

			assert.Equal(t, tt.expected, actual)
		})
	}
}

func Test_newConfig(t *testing.T) {
	t.Run("create config object", func(t *testing.T) {
		token := "token"
		u := "http://example.domain/api/users"

		_ = os.Setenv(TelegramBotToken, token)
		_ = os.Setenv(ConstructorUrl, u)

		expected := Config{
			telegramBotToken: token,
			constructorUrl:   u,
		}

		actual := newConfig()

		assert.Equal(t, expected, actual)
	})
}

func Test_newLogger(t *testing.T) {
	t.Run("successful created logger object", func(t *testing.T) {
		requestId := "requestId:1234567890"

		ctx := mocks.NewContextValue(t)
		ctx.EXPECT().Value("request_id").Return(requestId)

		actual := newLogger(ctx)
		expected := Logger{
			requestId: requestId,
			out:       os.Stdout,
		}

		assert.Equal(t, expected, actual)
	})

	type args struct {
		key   string
		value interface{}
	}
	failTests := []struct {
		name string
		args args
	}{
		{
			name: "fail incorrect type",
			args: args{
				key:   "request_id",
				value: 123,
			},
		},
		{
			name: "fail does not exist",
			args: args{
				key:   "request_id",
				value: nil,
			},
		},
	}
	for _, tt := range failTests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := mocks.NewContextValue(t)
			ctx.EXPECT().Value(tt.args.key).Return(tt.args.value)

			fn := func() { newLogger(ctx) }

			assert.PanicsWithValue(t, `The value "request_id" was not found in the context or it is not a string`, fn)
		})
	}
}

func Test_Main_integrate(t *testing.T) {
	init := map[string]url.Values{
		"correct": {
			"query_id":  {"query-id"},
			"user":      {`{"id":987654321,"first_name":"first-name","last_name":"last-name","username":"user-name","language_code":"xx"}`},
			"auth_date": {"1234567890"},
			"hash":      {"d2e2afac40ba23c950c636936d4a4c79b54ba9970555b0c8c5a87c48c834ab57"},
		},
		"incorrect-hash": {
			"query_id":  {"query-id"},
			"user":      {`{"id":987654321,"first_name":"first-name","last_name":"last-name","username":"user-name","language_code":"xx"}`},
			"auth_date": {"1234567890"},
			"hash":      {"hash"},
		},
		"wrong-type-user-id": {
			"user": []string{"%7B%22id%22%3Atrue%7D"},
			"hash": []string{"760692d2ce167e113854ba4602de84c75ca36be43d01d60e04d8628b9c260341"},
		},
	}

	type server struct {
		code int
		body []byte
	}

	type args struct {
		ctx    func() context.Context
		event  Event
		env    map[string]string
		server *server
	}

	tests := []struct {
		name     string
		args     args
		expected Response
	}{
		{
			name: "200 OK",
			args: args{
				ctx: func() context.Context {
					return context.WithValue(context.Background(), "request_id", "12345")
				},
				event: Event{
					Init:    init["correct"].Encode(),
					GroupId: "group-id:1234",
					Message: "message",
				},
				env: map[string]string{
					TelegramBotToken: TelegramBotToken,
				},
				server: &server{
					code: http.StatusOK,
					body: []byte(`{"ping-pong"}`),
				},
			},
			expected: Response{
				Body: ResponseBody{
					Status: ResponseStatusOk,
					Data:   `{"ping-pong"}`,
				},
				StatusCode: http.StatusOK,
				Headers: map[string]string{
					"Content-type": "application/json",
				},
			},
		},
		{
			name: "400 Bad Request. $.init not valid",
			args: args{
				ctx: func() context.Context {
					return context.WithValue(context.Background(), "request_id", "12345")
				},
				event: Event{
					Init: ";",
				},
				env: map[string]string{
					TelegramBotToken: TelegramBotToken,
				},
				server: nil,
			},
			expected: Response{
				Body: ResponseBody{
					Status:  ResponseStatusError,
					Message: "invalid semicolon separator in query",
				},
				StatusCode: http.StatusBadRequest,
				Headers: map[string]string{
					"Content-type": "application/json",
				},
			},
		},
		{
			name: "400 Bad Request. Error json parsing",
			args: args{
				ctx: func() context.Context {
					return context.WithValue(context.Background(), "request_id", "12345")
				},
				event: Event{
					Init: init["wrong-type-user-id"].Encode(),
				},
				env: map[string]string{
					TelegramBotToken: TelegramBotToken,
				},
				server: nil,
			},
			expected: Response{
				Body: ResponseBody{
					Status:  ResponseStatusError,
					Message: "invalid character '%' looking for beginning of value",
				},
				StatusCode: http.StatusBadRequest,
				Headers: map[string]string{
					"Content-type": "application/json",
				},
			},
		},
		{
			name: "403 Forbidden",
			args: args{
				ctx: func() context.Context {
					return context.WithValue(context.Background(), "request_id", "12345")
				},
				event: Event{
					Init: init["incorrect-hash"].Encode(),
				},
				env: map[string]string{
					TelegramBotToken: TelegramBotToken,
				},
				server: nil,
			},
			expected: Response{
				Body: ResponseBody{
					Status:  ResponseStatusError,
					Message: ResponseMessageAccessDenied,
				},
				StatusCode: http.StatusForbidden,
				Headers: map[string]string{
					"Content-type": "application/json",
				},
			},
		},
		{
			name: "502 BadGateway. Like connection refused",
			args: args{
				ctx: func() context.Context {
					return context.WithValue(context.Background(), "request_id", "12345")
				},
				event: Event{
					Init:    init["correct"].Encode(),
					GroupId: "group-id:1234",
					Message: "message",
				},
				env: map[string]string{
					TelegramBotToken: TelegramBotToken,
				},
				server: nil,
			},
			expected: Response{
				Body: ResponseBody{
					Status: ResponseStatusError,
				},
				StatusCode: http.StatusBadGateway,
				Headers: map[string]string{
					"Content-type": "application/json",
				},
			},
		},
		{
			name: "Status code != 200",
			args: args{
				ctx: func() context.Context {
					return context.WithValue(context.Background(), "request_id", "12345")
				},
				event: Event{
					Init:    init["correct"].Encode(),
					GroupId: "group-id:1234",
					Message: "message",
				},
				env: map[string]string{
					TelegramBotToken: TelegramBotToken,
				},
				server: &server{
					code: http.StatusTooManyRequests,
					body: []byte(`{"429 Too Many Requests"}`),
				},
			},
			expected: Response{
				Body: ResponseBody{
					Status: ResponseStatusNotOk,
					Data:   `{"429 Too Many Requests"}`,
				},
				StatusCode: http.StatusTooManyRequests,
				Headers: map[string]string{
					"Content-type": "application/json",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			constructUrl := "https://not-exist-d2e2afac40ba23c950c636936d4a4c.domain"

			if tt.args.server != nil {
				testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(tt.args.server.code)
					if tt.args.server.body != nil {
						_, _ = w.Write(tt.args.server.body)
					}
				}))
				defer testServer.Close()

				constructUrl = testServer.URL
			}

			tt.args.env[ConstructorUrl] = constructUrl
			for key, val := range tt.args.env {
				_ = os.Setenv(key, val)
			}

			assert.Equal(t, tt.expected, Main(tt.args.ctx(), tt.args.event))
		})
	}
}
