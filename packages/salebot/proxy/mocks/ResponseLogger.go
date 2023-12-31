// Code generated by mockery v2.34.1. DO NOT EDIT.

package mocks

import mock "github.com/stretchr/testify/mock"

// ResponseLogger is an autogenerated mock type for the ResponseLogger type
type ResponseLogger struct {
	mock.Mock
}

type ResponseLogger_Expecter struct {
	mock *mock.Mock
}

func (_m *ResponseLogger) EXPECT() *ResponseLogger_Expecter {
	return &ResponseLogger_Expecter{mock: &_m.Mock}
}

// Log provides a mock function with given fields: message
func (_m *ResponseLogger) Log(message []string) {
	_m.Called(message)
}

// ResponseLogger_Log_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Log'
type ResponseLogger_Log_Call struct {
	*mock.Call
}

// Log is a helper method to define mock.On call
//   - message []string
func (_e *ResponseLogger_Expecter) Log(message interface{}) *ResponseLogger_Log_Call {
	return &ResponseLogger_Log_Call{Call: _e.mock.On("Log", message)}
}

func (_c *ResponseLogger_Log_Call) Run(run func(message []string)) *ResponseLogger_Log_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].([]string))
	})
	return _c
}

func (_c *ResponseLogger_Log_Call) Return() *ResponseLogger_Log_Call {
	_c.Call.Return()
	return _c
}

func (_c *ResponseLogger_Log_Call) RunAndReturn(run func([]string)) *ResponseLogger_Log_Call {
	_c.Call.Return(run)
	return _c
}

// NewResponseLogger creates a new instance of ResponseLogger. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewResponseLogger(t interface {
	mock.TestingT
	Cleanup(func())
}) *ResponseLogger {
	mock := &ResponseLogger{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
