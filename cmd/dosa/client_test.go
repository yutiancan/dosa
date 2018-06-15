// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package main

import (
	"context"
	"reflect"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/uber-go/dosa"
	"github.com/uber-go/dosa/connectors/devnull"
	"github.com/uber-go/dosa/mocks"
)

type ClientTestEntity struct {
	dosa.Entity `dosa:"primaryKey=(ID)"`
	dosa.Index  `dosa:"key=Name, name=username"`
	ID          int64
	Name        string
	Email       string
}

var (
	table, _      = dosa.FindEntityByName(".", "ClientTestEntity")
	ctx           = context.TODO()
	scope         = "test"
	namePrefix    = "team.service"
	nullConnector = devnull.NewConnector()
	query1        = &queryObj{fieldName: "ID", colName: "id", op: "eq", valueStr: "10", value: dosa.FieldValue(int64(10))}
	query2        = &queryObj{fieldName: "ID", colName: "id", op: "lt", valueStr: "10", value: dosa.FieldValue(int64(10))}
	query3        = &queryObj{fieldName: "ID", colName: "id", op: "ne", valueStr: "10", value: dosa.FieldValue(int64(10))}
)

func TestNewClient(t *testing.T) {
	// initialize registrar
	reg, err := newSimpleRegistrar(scope, namePrefix, table)
	assert.NoError(t, err)
	assert.NotNil(t, reg)

	// initialize a pseudo-connected client
	client := newShellQueryClient(reg, nullConnector)
	err = client.Initialize(ctx)
	assert.NoError(t, err)
}

func TestClient_Initialize(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	reg, _ := newSimpleRegistrar(scope, namePrefix, table)

	// CheckSchema error
	errConn := mocks.NewMockConnector(ctrl)
	errConn.EXPECT().CheckSchema(ctx, gomock.Any(), gomock.Any(), gomock.Any()).Return(int32(-1), errors.New("CheckSchema error")).AnyTimes()
	c1 := newShellQueryClient(reg, errConn)
	assert.Error(t, c1.Initialize(ctx))

	// success case
	c2 := dosa.NewClient(reg, nullConnector)
	assert.NoError(t, c2.Initialize(ctx))
}

func TestClient_Read(t *testing.T) {
	reg, _ := newSimpleRegistrar(scope, namePrefix, table)
	fieldsToRead := []string{"ID", "Email"}
	results := map[string]dosa.FieldValue{
		"id":    int64(2),
		"name":  "bar",
		"email": "bar@email.com",
	}

	// success case
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockConn := mocks.NewMockConnector(ctrl)
	mockConn.EXPECT().CheckSchema(ctx, gomock.Any(), gomock.Any(), gomock.Any()).Return(int32(1), nil).AnyTimes()
	mockConn.EXPECT().Read(ctx, gomock.Any(), gomock.Any(), gomock.Any()).
		Do(func(_ context.Context, _ *dosa.EntityInfo, columnValues map[string]dosa.FieldValue, columnsToRead []string) {
			assert.Equal(t, dosa.FieldValue(int64(10)), columnValues["id"])
			assert.Equal(t, []string{"id", "email"}, columnsToRead)
		}).Return(results, nil).MinTimes(1)
	c := newShellQueryClient(reg, mockConn)
	assert.NoError(t, c.Initialize(ctx))
	fvs, err := c.Read(ctx, []*queryObj{query1}, fieldsToRead, 1)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(fvs))
	assert.Equal(t, results["id"], fvs[0]["ID"])
	assert.Equal(t, results["name"], fvs[0]["Name"])
	assert.Equal(t, results["email"], fvs[0]["Email"])

	// error in query, input non-supported operators
	fvs, err = c.Read(ctx, []*queryObj{query2}, fieldsToRead, 1)
	assert.Nil(t, fvs)
	assert.Contains(t, err.Error(), "wrong operator used for read")

	// error in column name converting
	fvs, err = c.Read(ctx, []*queryObj{query1}, []string{"badcol"}, 1)
	assert.Nil(t, fvs)
	assert.Contains(t, err.Error(), "badcol")
}

func TestClient_Range(t *testing.T) {
	reg, _ := newSimpleRegistrar(scope, namePrefix, table)
	fieldsToRead := []string{"ID", "Email"}
	results := map[string]dosa.FieldValue{
		"id":    int64(2),
		"name":  "bar",
		"email": "bar@email.com",
	}

	// success case
	resLimit := 10
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockConn := mocks.NewMockConnector(ctrl)
	mockConn.EXPECT().CheckSchema(ctx, gomock.Any(), gomock.Any(), gomock.Any()).Return(int32(1), nil).AnyTimes()
	mockConn.EXPECT().Range(ctx, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Do(func(_ context.Context, _ *dosa.EntityInfo, _ map[string][]*dosa.Condition, minimumFields []string, token string, limit int) {
			assert.Equal(t, "", token)
			assert.Equal(t, []string{"id", "email"}, minimumFields)
			assert.Equal(t, resLimit, limit)
		}).Return([]map[string]dosa.FieldValue{results}, "", nil).MinTimes(1)
	c := newShellQueryClient(reg, mockConn)
	assert.NoError(t, c.Initialize(ctx))
	fvs, err := c.Range(ctx, []*queryObj{query1, query2}, fieldsToRead, resLimit)
	assert.NoError(t, err)
	assert.NotNil(t, fvs)
	assert.Equal(t, 1, len(fvs))
	assert.Equal(t, results["id"], fvs[0]["ID"])
	assert.Equal(t, results["name"], fvs[0]["Name"])
	assert.Equal(t, results["email"], fvs[0]["Email"])

	// error in query, input non-supported operators
	fvs, err = c.Range(ctx, []*queryObj{query3}, fieldsToRead, resLimit)
	assert.Nil(t, fvs)
	assert.Contains(t, err.Error(), "wrong operator used for range")

	// error in column name converting
	fvs, err = c.Range(ctx, []*queryObj{query1}, []string{"badcol"}, resLimit)
	assert.Nil(t, fvs)
	assert.Contains(t, err.Error(), "badcol")
}

func TestClient_ColToFieldName(t *testing.T) {
	colToField := map[string]string{
		"id":    "ID",
		"name":  "Name",
		"email": "Email",
	}

	rowsCol := []map[string]dosa.FieldValue{
		{
			"id":   dosa.FieldValue(int(10)),
			"name": dosa.FieldValue("foo"),
		},
		// contains column "address" which not defined in struct
		{
			"id":      dosa.FieldValue(int(20)),
			"address": dosa.FieldValue("mars"),
		},
	}

	expRowsField := []map[string]dosa.FieldValue{
		{
			"ID":   dosa.FieldValue(int(10)),
			"Name": dosa.FieldValue("foo"),
		},
		// value of column "address" should not be returned
		{
			"ID": dosa.FieldValue(int(20)),
		},
	}

	rowsField := convertColToField(rowsCol, colToField)
	assert.True(t, reflect.DeepEqual(expRowsField, rowsField))

}

func TestClient_BuildReadArgs(t *testing.T) {
	// success case
	args, err := buildReadArgs([]*queryObj{query1})
	assert.NotNil(t, args)
	assert.NoError(t, err)
	fv, ok := args["id"]
	assert.True(t, ok)
	assert.Equal(t, dosa.FieldValue(int64(10)), fv)

	// fail case, input non-supported operator
	args, err = buildReadArgs([]*queryObj{query2})
	assert.Nil(t, args)
	assert.Contains(t, err.Error(), "wrong operator used for read")
}

func TestClient_BuildRangeOp(t *testing.T) {
	limit := 1

	// success case
	rop, err := buildRangeOp([]*queryObj{query1, query2}, limit)
	assert.NotNil(t, rop)
	assert.NoError(t, err)
	assert.Equal(t, limit, rop.LimitRows())
	conditions := rop.Conditions()
	assert.Len(t, conditions, 1)
	assert.Len(t, conditions["ID"], 2)

	// fail case, input non-supported operator
	rop, err = buildRangeOp([]*queryObj{query3}, limit)
	assert.Nil(t, rop)
	assert.Contains(t, err.Error(), "wrong operator used for range")
}
