package yarf

import (
	"errors"
	"strings"
)

// Router interface provides the methods used to handle route and routeGroup objects.
type Router interface {
	Match(string, *Context) bool
	Dispatch(*Context) error
}

// route struct stores the expected route path and the ResourceHandler that handles that route.
type route struct {
	path string // Original route

	parsed string // Cleaned route used to Match() against request url

	handler ResourceHandler // Handler for the route
}

// Route returns a new route object initialized with the provided data.
// Params:
//	- url string 		// The route path to handle
//	- h	ResourceHandler	// The ResourceHandler object that will process the requests to the url.
//
func Route(url string, h ResourceHandler) *route {
	r := new(route)
	r.path = url
	r.handler = h

	// Clean initial and trailing "/" from url
	for strings.HasPrefix(url, "/") {
		url = strings.TrimPrefix(url, "/")
	}
	for strings.HasSuffix(url, "/") {
		url = strings.TrimSuffix(url, "/")
	}
	r.parsed = url

	return r
}

// Match returns true/false indicating if a request URL matches the route.
// Route matchs are exact, that means, there are not optional parameters.
// To implement optional parameters you can define different routes handled by the same ResourceHandler.
// When a route matches the request URL, this method will parse and fill
// the parameters parsed during the process into the Context object.
func (r *route) Match(url string, c *Context) bool {
	// Init params
	params := make(map[string]string)

	// Clean initial and trailing "/" from request url
	for strings.HasPrefix(url, "/") {
		url = strings.TrimPrefix(url, "/")
	}
	for strings.HasSuffix(url, "/") {
		url = strings.TrimSuffix(url, "/")
	}

	// Split parts
	routeParts := strings.Split(r.parsed, "/")
	urlParts := strings.Split(url, "/")

	// Remove empty parts
	for i, p := range routeParts {
		if p == "" {
			routeParts = append(routeParts[:i], routeParts[i+1:]...)
		}
	}
	for i, p := range urlParts {
		if p == "" {
			urlParts = append(urlParts[:i], urlParts[i+1:]...)
		}
	}

	// YARF router only accepts exact route matches, so check for part count.
	if len(urlParts) != len(routeParts) {
		return false
	}

	// Check for param matching
	if r.parsed != url {
		for i, r := range routeParts {
			// Check part
			if r != urlParts[i] && r[:1] != ":" {
				return false
			}

			// Check param
			if r[:1] == ":" {
				params[r[1:]] = urlParts[i]
			}
		}
	}

	// Success match. Store params and return true.
	for key, value := range params {
		c.Params.Set(key, value)
	}
	return true
}

// Dispatch executes the right ResourceHandler method based on the HTTP request in the Context object.
// Accepts HTTP method override, based on request header: X-HTTP-Method-Override
func (r *route) Dispatch(c *Context) (err error) {
	// Get HTTP method requested
	method := strings.ToUpper(c.Request.Method)

	// Check for method overriding
	mo := strings.ToUpper(c.Request.Header.Get("X-HTTP-Method-Override"))
	if mo != "" {
		method = mo
	}

	// Add Context to handler
	r.handler.SetContext(c)

	// Method dispatch
	switch method {
	case "GET":
		err = r.handler.Get()

	case "POST":
		err = r.handler.Post()

	case "PUT":
		err = r.handler.Put()

	case "PATCH":
		err = r.handler.Patch()

	case "DELETE":
		err = r.handler.Delete()

	case "OPTIONS":
		err = r.handler.Options()

	case "HEAD":
		err = r.handler.Head()

	case "TRACE":
		err = r.handler.Trace()

	case "CONNECT":
		err = r.handler.Connect()
	}

	// Return error status
	return
}

// routeGroup stores routes grouped under a single url prefix.
type routeGroup struct {
	prefix string // The url prefix path for all routes in the group

	parsed string // Cleaned prefix used to Match() against request url

	middleware []MiddlewareHandler // Group middleware resources

	routes []Router // Group routes

	lastMatch Router // Stores last matched route to be dispatched.
}

// RouteGroup creates a new routeGroup object and initializes it with the provided url prefix.
// The object implements Router interface to being able to handle groups as routes.
// Groups can be nested into each other,
// so it's possible to add a routeGroup as a route inside another routeGroup.
// Includes methods to work with middleware.
func RouteGroup(url string) *routeGroup {
	r := new(routeGroup)
	r.prefix = url

	// Clean initial and trailing "/" from url
	for strings.HasPrefix(url, "/") {
		url = strings.TrimPrefix(url, "/")
	}
	for strings.HasSuffix(url, "/") {
		url = strings.TrimSuffix(url, "/")
	}
	r.parsed = url

	return r
}

// Match loops through all routes inside the group and find for one that matches the request.
// After a match is found, the route matching is stored into lastMatch
// to being able to dispatch it directly after a match without looping again.
// Outside the box, works exactly the same as route.Match()
func (g *routeGroup) Match(url string, c *Context) bool {
	// Init group params
	params := make(map[string]string)

	// Clean initial and trailing "/" from request url
	for strings.HasPrefix(url, "/") {
		url = strings.TrimPrefix(url, "/")
	}
	for strings.HasSuffix(url, "/") {
		url = strings.TrimSuffix(url, "/")
	}

	// Split parts
	routeParts := strings.Split(g.parsed, "/")
	urlParts := strings.Split(url, "/")

	// Remove empty parts
	for i, p := range routeParts {
		if p == "" {
			routeParts = append(routeParts[:i], routeParts[i+1:]...)
		}
	}
	for i, p := range urlParts {
		if p == "" {
			urlParts = append(urlParts[:i], urlParts[i+1:]...)
		}
	}

	// Check for enough parts on the request
	if len(urlParts) < len(routeParts) {
		return false
	}

	// Check for param matching
	for i, p := range routeParts {
		// Check part
		if p != urlParts[i] && p[:1] != ":" {
			return false
		}

		// Check param
		if p[:1] == ":" {
			params[p[1:]] = urlParts[i]
		}
	}

	// Success match. Store group params.
	for key, value := range params {
		c.Params.Set(key, value)
	}

	// Remove prefix part form the request URL
	rUrl := strings.Join(urlParts[len(routeParts):], "/")

	// Now look for a match inside the routes collection
	for _, r := range g.routes {
		if r.Match(rUrl, c) {
			// If a match is found, store the lastMatch and return true.
			g.lastMatch = r
			return true
		}
	}

	// If no match found in this group, return false
	return false
}

// Dispatch loops through all routes inside the group and dispatch the one that matches the request.
// Outside the box, works exactly the same as route.Dispatch().
func (g *routeGroup) Dispatch(c *Context) (err error) {
	if g.lastMatch == nil {
		return errors.New("No matching route found")
	}

	// Pre-dispatch middleware
	for _, m := range g.middleware {
		// Add context to middleware
		m.SetContext(c)

		// Dispatch
		err = m.PreDispatch()
		if err != nil {
			return
		}
	}

	// Dispatch route
	err = g.lastMatch.Dispatch(c)
	if err != nil {
		return
	}

	// Post-dispatch middleware
	for _, m := range g.middleware {
		// Dispatch
		err = m.PostDispatch()
		if err != nil {
			return
		}
	}

	// Return success
	return
}

// Add inserts a new resource with it's associated route into the group object.
func (g *routeGroup) Add(url string, h ResourceHandler) {
	g.routes = append(g.routes, Route(url, h))
}

// AddGroup inserts a route group into the routes list of the group object.
// This makes possible to nest groups.
func (g *routeGroup) AddGroup(r *routeGroup) {
	g.routes = append(g.routes, r)
}

// Insert adds a MiddlewareHandler into the middleware list of the group object.
func (g *routeGroup) Insert(m MiddlewareHandler) {
	g.middleware = append(g.middleware, m)
}