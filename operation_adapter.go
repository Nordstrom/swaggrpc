// This contains operationAdapter, a type that adapts a single swagger operation (path + HTTP
// method) to a gRPC method.
//
// It also contains helper functions for serializing protocol buffers to and from swagger requests.

package swaggrpc

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/go-openapi/runtime"
	runtimeclient "github.com/go-openapi/runtime/client"
	"github.com/go-openapi/spec"
	"github.com/go-openapi/strfmt"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/protoc-gen-go/descriptor"

	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/dynamic"

	"google.golang.org/grpc"
)

// A function to write a single parameter value from a proto message to a swagger request.
type swaggerParamWriter func(*dynamic.Message, runtime.ClientRequest) error

// Constant unmarshaller, configured to be lenient with respect to extra JSON values.
var permissiveJSONUnmarshaler jsonpb.Unmarshaler = jsonpb.Unmarshaler{AllowUnknownFields: true}

// Constant no-op AuthWriter, needed for the go-openapi client.
var nopAuthWriter runtime.ClientAuthInfoWriterFunc = func(runtime.ClientRequest, strfmt.Registry) error {
	return nil
}

// A wrapper for a single operation in a swagger service. This matches a path+method in a swagger
// definition, and is exposed as a single gRPC method.
type operationAdapter struct {
	// The HTTP client to use. This is shared among all services on a gRPCProxy.
	httpClient *http.Client
	// The swagger client to use. This is shared among all endpoints in a swaggerService.
	swaggerClient *runtimeclient.Runtime
	// The HTTP method this endpoint talks on.
	httpMethod string
	// Swagger path definition for this endpoint. This may contain path templates, and may not contain
	// query strings.
	swaggerPath string
	// All parameter writer functions to serialize a request.
	paramWriters []swaggerParamWriter
	// The proto message type this receives as input.
	inputProtoType *desc.MessageDescriptor
	// The proto message type this returns as output.
	outputProtoType *desc.MessageDescriptor
}

// Construct a new endpoint from the given swagger & proto method descriptions.
func newPathWrapper(
	httpClient *http.Client,
	swaggerClient *runtimeclient.Runtime,
	httpMethod string,
	swaggerPath string,
	parameters map[string]*spec.Parameter,
	method *desc.MethodDescriptor,
) (*operationAdapter, error) {
	inputProtoType := method.GetInputType()
	newValue := &operationAdapter{
		httpClient:      httpClient,
		swaggerClient:   swaggerClient,
		httpMethod:      httpMethod,
		swaggerPath:     swaggerPath,
		paramWriters:    make([]swaggerParamWriter, 0, len(parameters)),
		inputProtoType:  inputProtoType,
		outputProtoType: method.GetOutputType(),
	}

	for _, param := range parameters {
		// Look up the field for this input proto.
		// TODO(jkinkead): Test the robustness of this.
		fieldName := strings.Replace(param.Name, "-", "_", -1)
		fieldDesc := inputProtoType.FindFieldByName(fieldName)
		if fieldDesc == nil {
			return nil, fmt.Errorf("Could not find proto field named %s", fieldName)
		}

		stringConverter, err := getStringConverter(fieldDesc, param)
		if err != nil {
			return nil, err
		}
		paramWriter, err := getParamWriter(param)
		if err != nil {
			return nil, err
		}

		swaggerParamWriter := func(message *dynamic.Message, request runtime.ClientRequest) error {
			stringValues := convertValues(message, fieldDesc, stringConverter)
			return paramWriter(stringValues, request)
		}

		newValue.paramWriters = append(newValue.paramWriters, swaggerParamWriter)
	}

	return newValue, nil
}

// Returns a serializer function for a given proto field / parameter pair.
func getStringConverter(fieldDesc *desc.FieldDescriptor, param *spec.Parameter) (func(interface{}) string, error) {
	// Maps are a special-case: openapi2proto only creates string-keyed maps, which means we can
	// easily serialize directly to JSON. Maps interally aren't a FieldDescriptorProto_TYPE, though -
	// they're an option on the field, so they're handled here.
	if fieldDesc.IsMap() {
		return func(value interface{}) string {
			mapValue, ok := value.(map[interface{}]interface{})
			if !ok {
				log.Print("ERROR: Non-map value passed to map converter.")
				return ""
			}
			convertedValue := make(map[string]interface{}, len(mapValue))
			for key, value := range mapValue {
				keyString, ok := key.(string)
				if !ok {
					log.Print("ERROR: Non-string key passed to map converter.")
					return ""
				}
				convertedValue[keyString] = value
			}
			bytes, err := json.Marshal(convertedValue)
			if err != nil {
				log.Printf("WARNING: Error JSON serializing: %s.", err)
				return ""
			}
			return string(bytes)
		}, nil
	}

	switch fieldDesc.GetType() {
	case descriptor.FieldDescriptorProto_TYPE_MESSAGE:
		// For Swagger 2.0, this should work in all cases where the parameter is a body parameter.
		// The specification is pretty quiet on how non-primitive items should be formatted when passed
		// as non-body parameters.
		// This won't work for formData - but openapi2proto also doesn't handle this case, so we don't
		// have to worry about it here.
		// Swagger 3.0 has formatting options that this can wrong; specifically, you can specify a
		// "form" format for data instead of JSON.
		return func(value interface{}) string {
			bytes, err := json.Marshal(value)
			if err != nil {
				log.Print("WARNING: Error JSON serializing; ignoring.", err)
			}
			return string(bytes)
		}, nil
	case descriptor.FieldDescriptorProto_TYPE_BOOL,
	  descriptor.FieldDescriptorProto_TYPE_INT64, descriptor.FieldDescriptorProto_TYPE_UINT64,
		descriptor.FieldDescriptorProto_TYPE_INT32, descriptor.FieldDescriptorProto_TYPE_FIXED64,
		descriptor.FieldDescriptorProto_TYPE_FIXED32, descriptor.FieldDescriptorProto_TYPE_UINT32,
		descriptor.FieldDescriptorProto_TYPE_SFIXED32, descriptor.FieldDescriptorProto_TYPE_SFIXED64,
		descriptor.FieldDescriptorProto_TYPE_SINT32, descriptor.FieldDescriptorProto_TYPE_SINT64,
	  descriptor.FieldDescriptorProto_TYPE_DOUBLE, descriptor.FieldDescriptorProto_TYPE_FLOAT:
		// %v does what we want for numeric + boolean types.
		return func(value interface{}) string { return fmt.Sprintf("%v", value) }, nil
	case descriptor.FieldDescriptorProto_TYPE_STRING:
		return func(value interface{}) string { return value.(string) }, nil
	case descriptor.FieldDescriptorProto_TYPE_GROUP:
		// Groups are not handled; openapi2proto only generates proto3 files.
		return nil, fmt.Errorf("got proto2-only type 'group'")
	case descriptor.FieldDescriptorProto_TYPE_BYTES:
		// TODO(jkinkead): openapi2proto does not currently handle bytes; both 'byte' and 'binary'
		// formats are ignored. This is a bug, however, and we should handle bytes here.
		return nil, fmt.Errorf("bytes not implemented")
	case descriptor.FieldDescriptorProto_TYPE_ENUM:
		return func(value interface{}) string {
			// Enums are not reliably handled. openapi2proto will treat ANY enum validator
			// (http://json-schema.org/latest/json-schema-validation.html#rfc.section.6.23) as a set of
			// strings, even if they are refs to other schemas. Non-string values are simply ignored.
			// These values are translated lossily to an enum name in the proto file. In order to
			// serialize, we rely on the fact that these are in the same order in the schema as in the
			// proto file, and use the field value as an array index.
			rawValue := value.(int32)
			if rawValue >= int32(len(param.Enum)) {
				// This should not happen when proto & swagger are in sync. Default to a non-panic outcome
				// (empty string) in case of bad input.
				log.Printf("ERROR: raw enum value '%d' out-of-bounds for param %s", rawValue, param.Name)
				return ""
			}
			enumValue, ok := param.Enum[rawValue].(string)
			if !ok {
				// This should not happen - openapi2proto won't generate a proto enum in this case. Default
				// to a non-panic outcome (empty string) in case something changes.
				return ""
			}
			return enumValue
		}, nil
	default:
		return nil, fmt.Errorf("ERROR: unhandled field type %s", fieldDesc.GetType())
	}
}

// Converts a single or repeated message into a slice of serialized strings for the given field
// descriptor, converted using the given toString function.
// go-openapi parameter APIs operate in terms of lists of strings.
func convertValues(
	message *dynamic.Message,
	fieldDesc *desc.FieldDescriptor,
	toString func(interface{}) string,
) []string {
	rawValue := message.GetField(fieldDesc)
	var values []interface{}
	// Maps will report as repeated. This should ignore maps.
	if fieldDesc.IsRepeated() && !fieldDesc.IsMap() {
		values = rawValue.([]interface{})
	} else {
		values = []interface{}{rawValue}
	}
	stringValues := make([]string, len(values))
	for i, value := range values {
		stringValues[i] = toString(value)
	}
	return stringValues
}

// Returns a function which will write the given param to a request. The param will be passed into
// the function as an already-serialized string.
func getParamWriter(param *spec.Parameter) (func([]string, runtime.ClientRequest) error, error) {
	// Determine the parameter type, and return an appropriate serializer for it.
	switch param.In {
	case "query":
		return func(values []string, request runtime.ClientRequest) error {
			return request.SetQueryParam(param.Name, values...)
		}, nil
	case "header":
		return func(values []string, request runtime.ClientRequest) error {
			return request.SetHeaderParam(param.Name, values...)
		}, nil
	case "path":
		// NOTE: Some swagger files have "path" parameters that are actually in the query string. This
		// doesn't check for this case.
		return func(values []string, request runtime.ClientRequest) error {
			if len(values) > 1 {
				log.Printf("WARNING: parameter %s had multple values, only one allowed!", param.Name)
			}
			return request.SetPathParam(param.Name, values[0])
		}, nil
	case "body":
		// NOTE: This is for Swagger 2.0 only. Swagger 3.0 has the body defined elsewhere.
		return func(values []string, request runtime.ClientRequest) error {
			if len(values) > 1 {
				log.Printf("WARNING: parameter %s had multple values, only one allowed!", param.Name)
			}
			// go-openapi expects this to be either a Reader, or something with a configured Producer.
			return request.SetBodyParam(strings.NewReader(values[0]))
		}, nil
	case "formData":
		// This is not generated by openapi2proto.
		return nil, fmt.Errorf("formData parameters are not supported")
	case "cookie":
		// These are 3.0-only.
		return nil, fmt.Errorf("swagger 3.0 cookie parameters are not supported")
	default:
		return nil, fmt.Errorf("ERROR: Unknown parameter location %q for parameter %q",
			param.In, param.Name)
	}
}

// Returns a serializer function for the given message. This is used to send a request through the
// openapi-go library.
func (p *operationAdapter) getRequestWriter(msg *dynamic.Message) runtime.ClientRequestWriterFunc {
	return func(request runtime.ClientRequest, format strfmt.Registry) error {
		for _, writer := range p.paramWriters {
			err := writer(msg, request)
			if err != nil {
				return err
			}
		}
		return nil
	}
}

// The deserializer function for this endpoint. This implements runtime.ClientResponseReader.
func (p *operationAdapter) ReadResponse(
	response runtime.ClientResponse,
	consumer runtime.Consumer) (interface{}, error) {

	protoOut := dynamic.NewMessage(p.outputProtoType)

	err := permissiveJSONUnmarshaler.Unmarshal(response.Body(), protoOut)
	return protoOut, err
}

// Handles a single gRPC call by proxying to the underlying swagger service.
// Returns any error encountered.
func (p *operationAdapter) handleGRPCRequest(stream grpc.ServerStream) error {
	protoIn := dynamic.NewMessage(p.inputProtoType)
	err := stream.RecvMsg(protoIn)
	if err != nil {
		log.Printf("Error deserializing request: %s", err)
		return err
	}

	operation := runtime.ClientOperation{
		// This appears to be ignored client-side.
		ID:          "",
		Method:      p.httpMethod,
		PathPattern: p.swaggerPath,
		// TODO(jkinkead): Fix these two - they should be determinable from the spec.
		ConsumesMediaTypes: []string{"application/json"},
		ProducesMediaTypes: []string{"application/json"},
		// TODO(jkinkead): Fix this. It should be in the spec.
		Schemes:  []string{"http"},
		Params:   p.getRequestWriter(protoIn),
		Reader:   p,
		AuthInfo: nopAuthWriter,
		Context:  nil,
		Client:   p.httpClient,
	}

	result, err := p.swaggerClient.Submit(&operation)
	if err != nil {
		log.Printf("Got non-nil error: %s", err)
		return err
	}

	resultMessage, isOk := result.(*dynamic.Message)
	if !isOk {
		// Should not happen.
		return fmt.Errorf("could not cast to expected result type")
	}

	return stream.SendMsg(resultMessage)
}
