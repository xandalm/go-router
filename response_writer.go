package router

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math/bits"
	"net/http"
	"reflect"
)

type ResponseWriter interface {
	SetStatus(code int)
	SetHeader(key, value string)
	Write(v any) error
	WriteString(v any) error
	WriteJSON(v any) error
}

type responseWriter struct {
	rw http.ResponseWriter
}

func (rw *responseWriter) SetStatus(code int) {
	rw.rw.WriteHeader(code)
}

func (rw *responseWriter) SetHeader(key, value string) {
	rw.rw.Header().Set(key, value)
}

func u16b(v uint16) []byte {
	b := make([]byte, 2)
	binary.BigEndian.PutUint16(b, v)
	return b
}

func u32b(v uint32) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, v)
	return b
}

func u64b(v uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, v)
	return b
}

func (rw *responseWriter) write(bs []byte) error {
	_, err := rw.rw.Write(bs)
	return err
}

func (rw *responseWriter) writeInt(v any) (err error) {
	switch val := v.(type) {
	case int:
		switch bits.UintSize {
		case 64:
			err = rw.write(u64b(uint64(uint(val))))
		case 32:
			err = rw.write(u32b(uint32(uint(val))))
		}
	case uint:
		switch bits.UintSize {
		case 64:
			err = rw.write(u64b(uint64(val)))
		case 32:
			err = rw.write(u32b(uint32(val)))
		}
	case int8:
		err = rw.write([]byte{uint8(val)})
	case uint8:
		err = rw.write([]byte{val})
	case int16:
		err = rw.write(u16b(uint16(val)))
	case uint16:
		err = rw.write(u16b(val))
	case int32:
		err = rw.write(u32b(uint32(val)))
	case uint32:
		err = rw.write(u32b(val))
	case int64:
		err = rw.write(u64b(uint64(val)))
	case uint64:
		err = rw.write(u64b(val))
	}
	return
}

func (rw *responseWriter) Write(v any) (err error) {
	val := reflect.ValueOf(v)
	for val.Kind() == reflect.Pointer {
		val = reflect.Indirect(val)
	}
	switch val.Kind() {
	case reflect.String:
		err = rw.write([]byte(val.String()))
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		err = rw.writeInt(val.Interface())
	default:
		err = fmt.Errorf("can't write %T type, must be a primitive or a pointer that refers to an instantiated primitive", v)
	}
	return
}

func (rw *responseWriter) WriteString(v any) error {
	_, err := fmt.Fprint(rw.rw, v)
	return err
}

func (rw *responseWriter) WriteJSON(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		panic(jsonEncodeError(v))
	}
	return rw.WriteString(string(data))
}

func jsonEncodeError(v any) string {
	return fmt.Sprintf("router: unable to represent %v as a json", v)
}
