package web

import (
	"bytes"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

const livereloadScript = `<script>
(function(){
  var ws = new WebSocket("ws://" + location.host + "/_livereload");
  ws.onclose = function() {
    setTimeout(function(){ location.reload(); }, 1000);
  };
})();
</script>`

// livereloadMiddleware injects a livereload script before </body> in HTML responses.
func livereloadMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Capture the response
			rec := &responseRecorder{
				ResponseWriter: c.Response().Writer,
				body:           &bytes.Buffer{},
			}
			c.Response().Writer = rec

			if err := next(c); err != nil {
				return err
			}

			body := rec.body.String()
			contentType := c.Response().Header().Get("Content-Type")

			if strings.Contains(contentType, "text/html") && strings.Contains(body, "</body>") {
				body = strings.Replace(body, "</body>", livereloadScript+"</body>", 1)
			}

			c.Response().Writer = rec.ResponseWriter
			c.Response().Header().Set("Content-Length", "")
			_, err := c.Response().Writer.Write([]byte(body))
			return err
		}
	}
}

type responseRecorder struct {
	http.ResponseWriter
	body       *bytes.Buffer
	statusCode int
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	return r.body.Write(b)
}

func (r *responseRecorder) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}
