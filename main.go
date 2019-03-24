// This is mostly based off of the kubernetes and repl examples from
// https://github.com/stripe/skycfg/tree/master/_examples/
package main

import (
	"context"
	"fmt"
	"reflect"

	docopt "github.com/docopt/docopt-go"
	gogo_jsonpb "github.com/gogo/protobuf/jsonpb"
	"go.starlark.net/starlark"

	"github.com/gogo/protobuf/jsonpb"

	// proto seems to provide code gen for importing
	// messages, and jsonpb provides marshalling that yaml can use
	gogo_proto "github.com/gogo/protobuf/proto"
	"github.com/stripe/skycfg/gogocompat"

	"github.com/golang/protobuf/descriptor"
	"github.com/golang/protobuf/proto"
	_ "github.com/golang/protobuf/ptypes/wrappers"
	"github.com/stripe/skycfg"
	yaml "gopkg.in/yaml.v2"

	_ "github.com/envoyproxy/go-control-plane/envoy/admin/v2alpha"
	_ "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	_ "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v2"
	_ "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
)

// From https://github.com/stripe/skycfg/issues/29
func fnGogoProtoFromJSON(t *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var msgType starlark.Value
	var value starlark.String
	if err := starlark.UnpackPositionalArgs("gogo_proto.from_json", args, kwargs, 2, &msgType, &value); err != nil {
		return nil, err
	}

	name := msgType.(starlark.Callable).Name()
	goType := gogo_proto.MessageType(name)
	if goType == nil {
		return nil, fmt.Errorf("TypeError: type `%s' not found", name)
	}

	var emptyMsg descriptor.Message
	if goType.Kind() == reflect.Ptr {
		goValue := reflect.New(goType.Elem()).Interface()
		if iface, ok := goValue.(descriptor.Message); ok {
			emptyMsg = iface
		}
	}
	if emptyMsg == nil {
		// Return a slightly useful error in case some clever person has
		// manually registered a `proto.Message` that doesn't use pointer
		// receivers.
		return nil, fmt.Errorf("InternalError: %v is not a generated proto.Message", goType)
	}

	msg := proto.Clone(emptyMsg)
	msg.Reset()

	if err := gogo_jsonpb.UnmarshalString(string(value), msg); err != nil {
		return nil, err
	}
	return skycfg.NewProtoMessage(msg), nil
}

func main() {

	usage := `followprotocol: lets you create envoy configurations based on the envoy API 

Usage:
  followprotocol <input-file>
  followprotocol -h | --help

Options:
  <input-file>  skycfg file to be read
  -h --help     Show this message.`

	arguments, _ := docopt.ParseDoc(usage)

	ctx := context.Background()
	// Load gogo_from_json into the starlark environment
	gogoFromJSONOpt := skycfg.WithGlobals(starlark.StringDict{"gogo_from_json": starlark.NewBuiltin("gogo_from_json", fnGogoProtoFromJSON)})
	protoRegistryOpt := skycfg.WithProtoRegistry(gogocompat.ProtoRegistry())
	config, err := skycfg.Load(ctx, arguments["<input-file>"].(string), gogoFromJSONOpt, protoRegistryOpt)
	if err != nil {
		panic(err)
	}
	messages, err := config.Main(ctx)
	if err != nil {
		panic(err)
	}
	for _, msg := range messages {
		var jsonMarshaler = &jsonpb.Marshaler{OrigName: true}

		marshaled, err := jsonMarshaler.MarshalToString(msg)
		sep := ""
		var yamlMap yaml.MapSlice
		if err := yaml.Unmarshal([]byte(marshaled), &yamlMap); err != nil {
			panic(fmt.Sprintf("yaml.Unmarshal: %v", err))
		}
		yamlMarshaled, err := yaml.Marshal(yamlMap)
		if err != nil {
			panic(fmt.Sprintf("yaml.Marshal: %v", err))
		}
		marshaled = string(yamlMarshaled)
		sep = "---\n"
		fmt.Printf("%s%s\n", sep, marshaled)
	}
}
