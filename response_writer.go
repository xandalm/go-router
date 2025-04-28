package router

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/bits"
	"net/http"
	"reflect"
)

var ErrHeterogenicTypeWhileWriting = errors.New("router: only homogeneous slice or array can be written")

// ResponseWriter has a embedded [http.ResponseWriter]
type ResponseWriter interface {
	// SetStatus calls to WriteHeader method from the
	// [http.ResponseWriter] to modifies the http response
	// status code.
	SetStatus(code int)
	// SetHeader calls to Set method from the [http.Header]
	// type held by [http.ResponseWriter] to modifies a http
	// response header.
	SetHeader(key, value string)
	// Sends byte data from the given value.
	//
	// The given value must be from a basic type like
	// numerical, string, boolean or homogeneous array/slice.
	// The given value will be converted to its byte(s) form.
	//
	// The Content-Type attribute from Header will be set to
	// application/octet-stream.
	//
	// Finally, writes the response body and complete the http reply.
	Send(v any) error
	// Sends the given string data.
	//
	// The Content-Type attribute from Header will be set to
	// text/plain.
	//
	// Finally, writes the response body and complete the http reply.
	SendString(v string) error
	// Sends the given value in its JSON representation.
	//
	// The method tries to write the given value in a JSON
	// format using [json.Marshal].
	// So, it's possible to customize the JSON representation
	// implementing the [json.Marshaler] interface to the sending type.
	//
	// The Content-Type attribute from Header will be set to
	// application/json.
	//
	// Finally, writes the response body and complete the http reply.
	SendJSON(v any) error
	// Sends the given string data.
	//
	// The Content-Type attribute from Header will be set to
	// text/html.
	//
	// Finally, writes the response body and complete the http reply.
	SendHTML(v string) error
}

type responseWriter struct {
	http.ResponseWriter
}

func (rw *responseWriter) SetStatus(code int) {
	rw.WriteHeader(code)
}

func (rw *responseWriter) SetHeader(key, value string) {
	rw.Header().Set(key, value)
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

func write(w io.Writer, b []byte) error {
	_, err := w.Write(b)
	if err != nil {
		return fmt.Errorf("router: unable to write data, %v", err)
	}
	return nil
}

func writeString(w io.Writer, s string) error {
	_, err := fmt.Fprint(w, s)
	if err != nil {
		return fmt.Errorf("router: unable to write data, %v", err)
	}
	return err
}

func writeInt(w io.Writer, v any) (err error) {
	switch val := v.(type) {
	case int:
		switch bits.UintSize {
		case 64:
			err = write(w, u64b(uint64(uint(val))))
		case 32:
			err = write(w, u32b(uint32(uint(val))))
		}
	case uint:
		switch bits.UintSize {
		case 64:
			err = write(w, u64b(uint64(val)))
		case 32:
			err = write(w, u32b(uint32(val)))
		}
	case int8:
		err = write(w, []byte{uint8(val)})
	case uint8:
		err = write(w, []byte{val})
	case int16:
		err = write(w, u16b(uint16(val)))
	case uint16:
		err = write(w, u16b(val))
	case int32:
		err = write(w, u32b(uint32(val)))
	case uint32:
		err = write(w, u32b(val))
	case int64:
		err = write(w, u64b(uint64(val)))
	case uint64:
		err = write(w, u64b(val))
	}
	return
}

func writeFloat(w io.Writer, v any) (err error) {
	switch val := v.(type) {
	case float32:
		err = write(w, u32b(math.Float32bits(val)))
	case float64:
		err = write(w, u64b(math.Float64bits(val)))
	}
	return
}

func bytestream[A any](collec []A, converter func(A) []byte) []byte {
	b := []byte{}
	for i := range len(collec) {
		b = append(b, converter(collec[i])...)
	}
	return b
}

func indirectSliceWrite(w io.Writer, v any) (err error) {
	collec := reflect.ValueOf(v)

	if collec.Len() == 0 {
		return
	}

	first := collec.Index(0)

	// count how many steps is needed to reach at the final referred value
	steps := 0
	for first.Kind() == reflect.Pointer {
		first = reflect.Indirect(first)
		steps++
	}

	expectedKind := first.Kind()

	var fn func(v reflect.Value) error
	buf := bytes.NewBuffer([]byte{})
	switch expectedKind {
	case reflect.String:
		fn = func(v reflect.Value) error {
			return write(buf, []byte(v.String()))
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		fn = func(v reflect.Value) error {
			return writeInt(buf, v.Interface())
		}
	case reflect.Float32, reflect.Float64:
		fn = func(v reflect.Value) error {
			return writeFloat(buf, v.Interface())
		}
	}

	err = fn(first)
	for i := 1; i < collec.Len() && err == nil; i++ {
		val := collec.Index(i)
		for range steps {
			val = reflect.Indirect(val)
		}
		if val.Kind() != expectedKind {
			return ErrHeterogenicTypeWhileWriting
		}
		err = fn(val)
	}
	if err != nil {
		return
	}
	err = write(w, buf.Bytes())
	return
}

func writeSlice(w io.Writer, v any) (err error) {
	var b []byte
	switch val := v.(type) {
	case []int:
		switch bits.UintSize {
		case 64:
			b = bytestream(val, func(v int) []byte {
				return u64b(uint64(uint(v)))
			})
		case 32:
			b = bytestream(val, func(v int) []byte {
				return u32b(uint32(uint(v)))
			})
		}
	case []uint:
		switch bits.UintSize {
		case 64:
			b = bytestream(val, func(v uint) []byte {
				return u64b(uint64(v))
			})
		case 32:
			b = bytestream(val, func(v uint) []byte {
				return u32b(uint32(v))
			})
		}
	case []int8:
		b = bytestream(val, func(v int8) []byte {
			return []byte{byte(v)}
		})
	case []uint8:
		b = bytestream(val, func(v uint8) []byte {
			return []byte{v}
		})
	case []int16:
		b = bytestream(val, func(v int16) []byte {
			return u16b(uint16(v))
		})
	case []uint16:
		b = bytestream(val, func(v uint16) []byte {
			return u16b(v)
		})
	case []int32:
		b = bytestream(val, func(v int32) []byte {
			return u32b(uint32(v))
		})
	case []uint32:
		b = bytestream(val, func(v uint32) []byte {
			return u32b(v)
		})
	case []int64:
		b = bytestream(val, func(v int64) []byte {
			return u64b(uint64(v))
		})
	case []uint64:
		b = bytestream(val, func(v uint64) []byte {
			return u64b(v)
		})
	case []float32:
		b = bytestream(val, func(v float32) []byte {
			return u32b(math.Float32bits(v))
		})
	case []float64:
		b = bytestream(val, func(v float64) []byte {
			return u64b(math.Float64bits(v))
		})
	case []any:
		return ErrHeterogenicTypeWhileWriting
	default:
		return indirectSliceWrite(w, val)
	}
	return write(w, b)
}

func (rw *responseWriter) Send(v any) (err error) {
	val := reflect.ValueOf(v)

	switch val.Kind() {
	case reflect.String:
		err = writeString(rw, val.String())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		err = writeInt(rw, val.Interface())
	case reflect.Float32, reflect.Float64:
		err = writeFloat(rw, val.Interface())
	case reflect.Slice, reflect.Array:
		err = writeSlice(rw, val.Interface())
	default:
		err = fmt.Errorf("router: can't write %T type", v)
	}
	return
}

func (rw *responseWriter) SendString(v string) error {
	rw.ResponseWriter.Header().Set("Content-Type", "text/plain")
	return writeString(rw, v)
}

func (rw *responseWriter) SendJSON(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("router: unable to represent %v as a json", v)
	}
	rw.ResponseWriter.Header().Set("Content-Type", "application/json")
	return write(rw, data)
}

func (rw *responseWriter) SendHTML(v string) error {
	rw.ResponseWriter.Header().Set("Content-Type", "text/html")
	return writeString(rw, v)
}
