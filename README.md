# Go Router

It's based on standard lib HTTP ServerMux (Go 1.21). With new features:
- RegExp match based seek.
- Param based patterns in path analysis.
- Instead Handle/HandleFunc methods, now we have Use/UseFunc methods, this can deal with all HTTP methods.
- Now we also have:
  - Get/GetFunc, for only HTTP GET methods;
  - Post/PostFunc, for only HTTP POST methods;
  - Put/PutFunc, for only HTTP PUT methods;
  - Delete/DeleteFunc, for only HTTP DELETE methods.

## ResponseWriter

The router.ResponseWriter is a alias for http.ResponseWriter

## Request

The router.Request is a embedded http.Request, that can hold the params recognized in the request path.
Which you can get through Params()

This type has a ParseBodyInto(), that can parse request body into a variable of the type int, float, string or struct give as an argument.

### Let's see

A router configuration that exposes a endpoint with:

    ro.GetFunc("/admin/orgs/{id}", func(w router.ResponseWriter, r *router.Request) {  
      w.WriteHeader(http.StatusOK)
    })

The request (GET "example.com/admin/orgs/e503a") should matches the pattern above, making a router.Request that holds a map with the key-value { id: "e503a" }
