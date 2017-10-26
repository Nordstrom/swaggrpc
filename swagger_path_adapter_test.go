package swaggrpc

import (
	"bytes"
	"strings"
	"testing"

	"github.com/go-openapi/spec"
	"github.com/golang/protobuf/jsonpb"
	"github.com/jhump/protoreflect/dynamic"
	assertions "github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Tests that getStringConverter returns the correct JSON serializer for proto types.
func TestGetStringConverter(t *testing.T) {
	// Proto file to extract test fields from.
	protoContent := `
syntax = "proto3";

message SubMessage {
 string subValue = 1;
}

message TestMessage {
	bool boolValue = 1;
	string stringValue = 2;
	int32 int32Value = 3;
	int64 int64Value = 4;
	double doubleValue = 5;
	enum TestEnum {
		FIRST = 0;
		SECOND = 1;
	}
	TestEnum enumValue = 6;
	map<string, int32> mapValue = 7;
	SubMessage messageValue = 8;
}
`
	fileDesc, err := loadProtoFromBytes(([]byte)(protoContent))
	require.Nil(t, err, "Couldn't parse test fixture proto: %v", err)
	messageType := fileDesc.FindMessage("TestMessage")
	require.NotNil(t, messageType, "Couldn't find TestMessage in parsed proto")
	fixtures := []struct {
		fieldName   string
		parameter   *spec.Parameter
		textMessage string
		result      string
	}{
		{"boolValue", nil, `{"boolValue": true}`, "true"},
		{"stringValue", nil, `{"stringValue": "foo"}`, "foo"},
		{"int32Value", nil, `{"int32Value": 1234}`, "1234"},
		{"int64Value", nil, `{"int64Value": 4567}`, "4567"},
		{"doubleValue", nil, `{"doubleValue": 3.34}`, "3.34"},
		{"enumValue", (&spec.Parameter{}).WithEnum("first", "second"), `{"enumValue": "SECOND"}`, "second"},
		{"mapValue", nil, `{"mapValue": {"bar": 1, "foo": 2}}`, `{"bar":1,"foo":2}`},
		{"messageValue", nil, `{"messageValue": {"subValue": "str"}}`, `{"subValue":"str"}`},
	}
	for _, fixture := range fixtures {
		t.Run(strings.Title(fixture.fieldName), func(t *testing.T) {
			assert := assertions.New(t)
			fieldDesc := messageType.FindFieldByName(fixture.fieldName)
			converter, err := getStringConverter(fieldDesc, fixture.parameter)
			assert.Nil(err, "Error fetching converter: %v", err)
			assert.NotNil(converter, "Nil converter")
			if converter != nil {
				message := dynamic.NewMessage(messageType)
				err = jsonpb.Unmarshal(bytes.NewBuffer([]byte(fixture.textMessage)), message)
				assert.Nil(err, "Error unmarshaling text data: %s", err)
				result := converter(message.GetField(fieldDesc))
				assert.Equal(fixture.result, result, "Bad serialized value")
			}
		})
	}
}

// Tests that convertValues returns correct strings for repeated and non-repeated proto types.
func TestConvertValues(t *testing.T) {
	// Proto file to extract test fields from.
	protoContent := `
syntax = "proto3";

message TestMessage {
	string singleValue = 1;
	repeated string repeatedValue = 2;
	map<string, int32> mapValue = 3;
}
`
	fileDesc, err := loadProtoFromBytes(([]byte)(protoContent))
	require.Nil(t, err, "Couldn't parse test fixture proto: %v", err)
	messageType := fileDesc.FindMessage("TestMessage")
	require.NotNil(t, messageType, "Couldn't find TestMessage in parsed proto")
	fixtures := []struct {
		fieldName   string
		textMessage string
		result      []string
	}{
		{"singleValue", `{"singleValue": "foo"}`, []string{"foo"}},
		{"repeatedValue", `{"repeatedValue": ["foo", "bar", "gaz"]}`, []string{"foo", "bar", "gaz"}},
		{"mapValue", `{"mapValue":{"foo":1}}`, []string{`<map value>`}},
	}
	for _, fixture := range fixtures {
		t.Run(strings.Title(fixture.fieldName), func(t *testing.T) {
			assert := assertions.New(t)
			message := dynamic.NewMessage(messageType)
			err := jsonpb.Unmarshal(bytes.NewBuffer([]byte(fixture.textMessage)), message)
			assert.Nil(err, "Error unmarshaling text data: %v", err)
			fieldDesc := messageType.FindFieldByName(fixture.fieldName)
			result := convertValues(message, fieldDesc, func(input interface{}) string {
				// Test with an echoing toString function.
				stringValue, ok := input.(string)
				if ok {
					return stringValue
				}
				_, ok = input.(map[interface{}]interface{})
				if ok {
					return "<map value>"
				}
				return "UNKNOWN"
			})
			if assert.Equal(len(fixture.result), len(result), "Mismatched result length") {
				for i, item := range fixture.result {
					assert.Equal(item, result[i], "Bad result for item %d")
				}
			}
		})
	}
}
