# Go Router

Developed using standard http library in background.

New features that amplify ServerMux look-a-like structure. Allowing more flexibility implementing HTTP servers with routes and middlewares:
- Allow the creation of namespaces.
- Match searching by regular expression.
- Param recognition in the route pattern.
- **All** and **AllFunc** methods works as **Handle** and **HandleFunc** respectively, dealing with all HTTP methods.
- Methods to handle individual HTTP methods!
  - **Get**/**GetFunc**, for only GET methods;
  - **Post**/**PostFunc**, for only POST methods;
  - **Put**/**PutFunc**, for only PUT methods;
  - **Delete**/**DeleteFunc**, for only DELETE methods.
- **Use** and **UseFunc** allow to add middlewares to handle incoming requests.

## ResponseWriter

The **ResponseWriter** is an extension for **http.ResponseWriter**.

This type simplify methods to send data through the http, like **Send**, **SendString** and **SendJSON**.

The **Send** method should write the given data in a byte sequence, setting the "Content-Type" to 
"application/octet-stream". The **SendString** method should write the given data in a utf-8 string sequence.
The **SendJSON** method should write the given data in a string representing the JSON value, setting the "Content-Type" to "application/json".

The impossibility of converting any type that fits these methods writing will cause an error.

## Request

The **Request** is a embedded **http.Request**, that can hold the params recognized in the request path.
Which you can get through **Params** method

This type has a **BodyIn** method, that can parse request body into a variable of the type int, float, string or struct give as an argument.

## Diving into implementation

The router creation is simple:

    ro := router.NewRouter()

This method returns a instantiated **Router** reference (pointer)

The **Router** exposes methods for attaching a **Handler** to a path. There are methods that allow attaching a **Handler** adapted through a function. For example, the **Get** and **GetFunc** methods serve the same purpose, but **Get** accepts a **Handler** while **GetFunc** accepts a function with signature ```func(w router.ResponseWriter, r *router.Request)``` that will be adapted as a **Handler**.

With the router you can add simple routes, like:

    // Using All or AllFunc to handle all HTTP Request Methods
    ro.AllFunc("/all", func(w router.ResponseWriter, r *router.Request){})

    // Using Post or PostFunc to handle only HTTP POST Requests
    ro.PostFunc("/post", func(w router.ResponseWriter, r *router.Request){})

    // Using Get or GetFunc to handle only HTTP GET Requests
    ro.GetFunc("/get", func(w router.ResponseWriter, r *router.Request){})

    // Using Put or PutFunc to handle only HTTP Put Requests
    ro.PutFunc("/put", func(w router.ResponseWriter, r *router.Request){})

    // Using Delete or DeleteFunc to handle only HTTP DELETE Requests
    ro.DeleteFunc("/delete", func(w router.ResponseWriter, r *router.Request){})

Params are allowed in a path, they are defined between ```{}```, like ```"/users/{id}"```.

In a router configuration that exposes a endpoint with:

    ro.GetFunc("/admin/orgs/{id}", func(w router.ResponseWriter, r *router.Request) {  
      w.WriteHeader(http.StatusOK)
    })

The request ```GET http://example.com/admin/orgs/e503a``` should matches the pattern above, making a **Request** that holds a map with the key-value { id: "e503a" }

Namespaces can be added to router through **Namespace** method. It returns a instantiated **namespace** that expose methods similar to **Router** methods. With the returned value you can add nested routes, namespaces or middlewares.

    nsAdmin := ro.Namespace("admin")

    nsAdmin.Get("/users", func(w router.ResponseWriter, r *router.Request){ ... })
    nsAdmin.Get("/products", func(w router.ResponseWriter, r *router.Request){ ... })

    nsAdminMedia := nsAdmin.Namespace("media")
    ...

Namespace method also allows to pass a path that contains params markups, but the params markups will be transformed in a generic param. Like ```"/users/{id}"``` becomes ```"/users/{}"```. This means that one param doesn't make the namespace exclusive.
