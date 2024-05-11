// Code generated by mockery v2.43.0. DO NOT EDIT.

package mocks

import (
	context "context"

	s3 "github.com/aws/aws-sdk-go-v2/service/s3"
	mock "github.com/stretchr/testify/mock"
)

// MockAPIClient is an autogenerated mock type for the APIClient type
type MockAPIClient struct {
	mock.Mock
}

type MockAPIClient_Expecter struct {
	mock *mock.Mock
}

func (_m *MockAPIClient) EXPECT() *MockAPIClient_Expecter {
	return &MockAPIClient_Expecter{mock: &_m.Mock}
}

// CopyObject provides a mock function with given fields: ctx, params, optFns
func (_m *MockAPIClient) CopyObject(ctx context.Context, params *s3.CopyObjectInput, optFns ...func(*s3.Options)) (*s3.CopyObjectOutput, error) {
	_va := make([]interface{}, len(optFns))
	for _i := range optFns {
		_va[_i] = optFns[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, params)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	if len(ret) == 0 {
		panic("no return value specified for CopyObject")
	}

	var r0 *s3.CopyObjectOutput
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, *s3.CopyObjectInput, ...func(*s3.Options)) (*s3.CopyObjectOutput, error)); ok {
		return rf(ctx, params, optFns...)
	}
	if rf, ok := ret.Get(0).(func(context.Context, *s3.CopyObjectInput, ...func(*s3.Options)) *s3.CopyObjectOutput); ok {
		r0 = rf(ctx, params, optFns...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*s3.CopyObjectOutput)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, *s3.CopyObjectInput, ...func(*s3.Options)) error); ok {
		r1 = rf(ctx, params, optFns...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MockAPIClient_CopyObject_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'CopyObject'
type MockAPIClient_CopyObject_Call struct {
	*mock.Call
}

// CopyObject is a helper method to define mock.On call
//   - ctx context.Context
//   - params *s3.CopyObjectInput
//   - optFns ...func(*s3.Options)
func (_e *MockAPIClient_Expecter) CopyObject(ctx interface{}, params interface{}, optFns ...interface{}) *MockAPIClient_CopyObject_Call {
	return &MockAPIClient_CopyObject_Call{Call: _e.mock.On("CopyObject",
		append([]interface{}{ctx, params}, optFns...)...)}
}

func (_c *MockAPIClient_CopyObject_Call) Run(run func(ctx context.Context, params *s3.CopyObjectInput, optFns ...func(*s3.Options))) *MockAPIClient_CopyObject_Call {
	_c.Call.Run(func(args mock.Arguments) {
		variadicArgs := make([]func(*s3.Options), len(args)-2)
		for i, a := range args[2:] {
			if a != nil {
				variadicArgs[i] = a.(func(*s3.Options))
			}
		}
		run(args[0].(context.Context), args[1].(*s3.CopyObjectInput), variadicArgs...)
	})
	return _c
}

func (_c *MockAPIClient_CopyObject_Call) Return(_a0 *s3.CopyObjectOutput, _a1 error) *MockAPIClient_CopyObject_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *MockAPIClient_CopyObject_Call) RunAndReturn(run func(context.Context, *s3.CopyObjectInput, ...func(*s3.Options)) (*s3.CopyObjectOutput, error)) *MockAPIClient_CopyObject_Call {
	_c.Call.Return(run)
	return _c
}

// GetObject provides a mock function with given fields: ctx, params, optFns
func (_m *MockAPIClient) GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	_va := make([]interface{}, len(optFns))
	for _i := range optFns {
		_va[_i] = optFns[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, params)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	if len(ret) == 0 {
		panic("no return value specified for GetObject")
	}

	var r0 *s3.GetObjectOutput
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, *s3.GetObjectInput, ...func(*s3.Options)) (*s3.GetObjectOutput, error)); ok {
		return rf(ctx, params, optFns...)
	}
	if rf, ok := ret.Get(0).(func(context.Context, *s3.GetObjectInput, ...func(*s3.Options)) *s3.GetObjectOutput); ok {
		r0 = rf(ctx, params, optFns...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*s3.GetObjectOutput)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, *s3.GetObjectInput, ...func(*s3.Options)) error); ok {
		r1 = rf(ctx, params, optFns...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MockAPIClient_GetObject_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'GetObject'
type MockAPIClient_GetObject_Call struct {
	*mock.Call
}

// GetObject is a helper method to define mock.On call
//   - ctx context.Context
//   - params *s3.GetObjectInput
//   - optFns ...func(*s3.Options)
func (_e *MockAPIClient_Expecter) GetObject(ctx interface{}, params interface{}, optFns ...interface{}) *MockAPIClient_GetObject_Call {
	return &MockAPIClient_GetObject_Call{Call: _e.mock.On("GetObject",
		append([]interface{}{ctx, params}, optFns...)...)}
}

func (_c *MockAPIClient_GetObject_Call) Run(run func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options))) *MockAPIClient_GetObject_Call {
	_c.Call.Run(func(args mock.Arguments) {
		variadicArgs := make([]func(*s3.Options), len(args)-2)
		for i, a := range args[2:] {
			if a != nil {
				variadicArgs[i] = a.(func(*s3.Options))
			}
		}
		run(args[0].(context.Context), args[1].(*s3.GetObjectInput), variadicArgs...)
	})
	return _c
}

func (_c *MockAPIClient_GetObject_Call) Return(_a0 *s3.GetObjectOutput, _a1 error) *MockAPIClient_GetObject_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *MockAPIClient_GetObject_Call) RunAndReturn(run func(context.Context, *s3.GetObjectInput, ...func(*s3.Options)) (*s3.GetObjectOutput, error)) *MockAPIClient_GetObject_Call {
	_c.Call.Return(run)
	return _c
}

// GetObjectAcl provides a mock function with given fields: ctx, params, optFns
func (_m *MockAPIClient) GetObjectAcl(ctx context.Context, params *s3.GetObjectAclInput, optFns ...func(*s3.Options)) (*s3.GetObjectAclOutput, error) {
	_va := make([]interface{}, len(optFns))
	for _i := range optFns {
		_va[_i] = optFns[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, params)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	if len(ret) == 0 {
		panic("no return value specified for GetObjectAcl")
	}

	var r0 *s3.GetObjectAclOutput
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, *s3.GetObjectAclInput, ...func(*s3.Options)) (*s3.GetObjectAclOutput, error)); ok {
		return rf(ctx, params, optFns...)
	}
	if rf, ok := ret.Get(0).(func(context.Context, *s3.GetObjectAclInput, ...func(*s3.Options)) *s3.GetObjectAclOutput); ok {
		r0 = rf(ctx, params, optFns...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*s3.GetObjectAclOutput)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, *s3.GetObjectAclInput, ...func(*s3.Options)) error); ok {
		r1 = rf(ctx, params, optFns...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MockAPIClient_GetObjectAcl_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'GetObjectAcl'
type MockAPIClient_GetObjectAcl_Call struct {
	*mock.Call
}

// GetObjectAcl is a helper method to define mock.On call
//   - ctx context.Context
//   - params *s3.GetObjectAclInput
//   - optFns ...func(*s3.Options)
func (_e *MockAPIClient_Expecter) GetObjectAcl(ctx interface{}, params interface{}, optFns ...interface{}) *MockAPIClient_GetObjectAcl_Call {
	return &MockAPIClient_GetObjectAcl_Call{Call: _e.mock.On("GetObjectAcl",
		append([]interface{}{ctx, params}, optFns...)...)}
}

func (_c *MockAPIClient_GetObjectAcl_Call) Run(run func(ctx context.Context, params *s3.GetObjectAclInput, optFns ...func(*s3.Options))) *MockAPIClient_GetObjectAcl_Call {
	_c.Call.Run(func(args mock.Arguments) {
		variadicArgs := make([]func(*s3.Options), len(args)-2)
		for i, a := range args[2:] {
			if a != nil {
				variadicArgs[i] = a.(func(*s3.Options))
			}
		}
		run(args[0].(context.Context), args[1].(*s3.GetObjectAclInput), variadicArgs...)
	})
	return _c
}

func (_c *MockAPIClient_GetObjectAcl_Call) Return(_a0 *s3.GetObjectAclOutput, _a1 error) *MockAPIClient_GetObjectAcl_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *MockAPIClient_GetObjectAcl_Call) RunAndReturn(run func(context.Context, *s3.GetObjectAclInput, ...func(*s3.Options)) (*s3.GetObjectAclOutput, error)) *MockAPIClient_GetObjectAcl_Call {
	_c.Call.Return(run)
	return _c
}

// HeadObject provides a mock function with given fields: ctx, params, optFns
func (_m *MockAPIClient) HeadObject(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
	_va := make([]interface{}, len(optFns))
	for _i := range optFns {
		_va[_i] = optFns[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, params)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	if len(ret) == 0 {
		panic("no return value specified for HeadObject")
	}

	var r0 *s3.HeadObjectOutput
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, *s3.HeadObjectInput, ...func(*s3.Options)) (*s3.HeadObjectOutput, error)); ok {
		return rf(ctx, params, optFns...)
	}
	if rf, ok := ret.Get(0).(func(context.Context, *s3.HeadObjectInput, ...func(*s3.Options)) *s3.HeadObjectOutput); ok {
		r0 = rf(ctx, params, optFns...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*s3.HeadObjectOutput)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, *s3.HeadObjectInput, ...func(*s3.Options)) error); ok {
		r1 = rf(ctx, params, optFns...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MockAPIClient_HeadObject_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'HeadObject'
type MockAPIClient_HeadObject_Call struct {
	*mock.Call
}

// HeadObject is a helper method to define mock.On call
//   - ctx context.Context
//   - params *s3.HeadObjectInput
//   - optFns ...func(*s3.Options)
func (_e *MockAPIClient_Expecter) HeadObject(ctx interface{}, params interface{}, optFns ...interface{}) *MockAPIClient_HeadObject_Call {
	return &MockAPIClient_HeadObject_Call{Call: _e.mock.On("HeadObject",
		append([]interface{}{ctx, params}, optFns...)...)}
}

func (_c *MockAPIClient_HeadObject_Call) Run(run func(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options))) *MockAPIClient_HeadObject_Call {
	_c.Call.Run(func(args mock.Arguments) {
		variadicArgs := make([]func(*s3.Options), len(args)-2)
		for i, a := range args[2:] {
			if a != nil {
				variadicArgs[i] = a.(func(*s3.Options))
			}
		}
		run(args[0].(context.Context), args[1].(*s3.HeadObjectInput), variadicArgs...)
	})
	return _c
}

func (_c *MockAPIClient_HeadObject_Call) Return(_a0 *s3.HeadObjectOutput, _a1 error) *MockAPIClient_HeadObject_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *MockAPIClient_HeadObject_Call) RunAndReturn(run func(context.Context, *s3.HeadObjectInput, ...func(*s3.Options)) (*s3.HeadObjectOutput, error)) *MockAPIClient_HeadObject_Call {
	_c.Call.Return(run)
	return _c
}

// PutObject provides a mock function with given fields: ctx, params, optFns
func (_m *MockAPIClient) PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	_va := make([]interface{}, len(optFns))
	for _i := range optFns {
		_va[_i] = optFns[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, params)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	if len(ret) == 0 {
		panic("no return value specified for PutObject")
	}

	var r0 *s3.PutObjectOutput
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, *s3.PutObjectInput, ...func(*s3.Options)) (*s3.PutObjectOutput, error)); ok {
		return rf(ctx, params, optFns...)
	}
	if rf, ok := ret.Get(0).(func(context.Context, *s3.PutObjectInput, ...func(*s3.Options)) *s3.PutObjectOutput); ok {
		r0 = rf(ctx, params, optFns...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*s3.PutObjectOutput)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, *s3.PutObjectInput, ...func(*s3.Options)) error); ok {
		r1 = rf(ctx, params, optFns...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MockAPIClient_PutObject_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'PutObject'
type MockAPIClient_PutObject_Call struct {
	*mock.Call
}

// PutObject is a helper method to define mock.On call
//   - ctx context.Context
//   - params *s3.PutObjectInput
//   - optFns ...func(*s3.Options)
func (_e *MockAPIClient_Expecter) PutObject(ctx interface{}, params interface{}, optFns ...interface{}) *MockAPIClient_PutObject_Call {
	return &MockAPIClient_PutObject_Call{Call: _e.mock.On("PutObject",
		append([]interface{}{ctx, params}, optFns...)...)}
}

func (_c *MockAPIClient_PutObject_Call) Run(run func(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options))) *MockAPIClient_PutObject_Call {
	_c.Call.Run(func(args mock.Arguments) {
		variadicArgs := make([]func(*s3.Options), len(args)-2)
		for i, a := range args[2:] {
			if a != nil {
				variadicArgs[i] = a.(func(*s3.Options))
			}
		}
		run(args[0].(context.Context), args[1].(*s3.PutObjectInput), variadicArgs...)
	})
	return _c
}

func (_c *MockAPIClient_PutObject_Call) Return(_a0 *s3.PutObjectOutput, _a1 error) *MockAPIClient_PutObject_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *MockAPIClient_PutObject_Call) RunAndReturn(run func(context.Context, *s3.PutObjectInput, ...func(*s3.Options)) (*s3.PutObjectOutput, error)) *MockAPIClient_PutObject_Call {
	_c.Call.Return(run)
	return _c
}

// NewMockAPIClient creates a new instance of MockAPIClient. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewMockAPIClient(t interface {
	mock.TestingT
	Cleanup(func())
}) *MockAPIClient {
	mock := &MockAPIClient{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
