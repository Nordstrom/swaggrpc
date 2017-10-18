package swaggrpc

import (
	"bytes"
	"strings"
	"testing"

	"github.com/go-openapi/spec"

	"github.com/golang/protobuf/jsonpb"

	"github.com/jhump/protoreflect/dynamic"
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
	if err != nil {
		t.Fatalf("Couldn't parse test fixture proto: %s", err)
	}
	messageType := fileDesc.FindMessage("TestMessage")
	if messageType == nil {
		t.Fatal("Couldn't find TestMessage")
	}
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
			fieldDesc := messageType.FindFieldByName(fixture.fieldName)
			converter, err := getStringConverter(fieldDesc, fixture.parameter)
			if err != nil {
				t.Errorf("Got non-nil error fetching converter: %s", err)
			}
			if converter != nil {
				message := dynamic.NewMessage(messageType)
				err = jsonpb.Unmarshal(bytes.NewBuffer([]byte(fixture.textMessage)), message)
				if err != nil {
					t.Errorf("Error unmarshaling text data: %s", err)
				}
				result := converter(message.GetField(fieldDesc))
				if result != fixture.result {
					t.Errorf("Got serialized value %q; expected %q.", result, fixture.result)
				}
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
	if err != nil {
		t.Fatal("Couldn't parse test fixture proto")
	}
	messageType := fileDesc.FindMessage("TestMessage")
	if messageType == nil {
		t.Fatal("Couldn't find TestMessage")
	}
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
			message := dynamic.NewMessage(messageType)
			err := jsonpb.Unmarshal(bytes.NewBuffer([]byte(fixture.textMessage)), message)
			if err != nil {
				t.Errorf("Error unmarshaling text data: %s", err)
			}
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
			if len(fixture.result) != len(result) {
				t.Errorf("Expected %d results; got %d", len(fixture.result), len(result))
			} else {
				for i, item := range fixture.result {
					if item != result[i] {
						t.Errorf("For item %d, expected %q, got %q", i, item, result[i])
					}
				}
			}
		})
	}
}
