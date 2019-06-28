package private

import (
	"net"
	"net/http"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"

	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"github.com/keratin/authn-server/app"
	authnpb "github.com/keratin/authn-server/grpc"
	"github.com/keratin/authn-server/grpc/internal/gateway"
	"github.com/keratin/authn-server/grpc/public"
	"golang.org/x/net/context"
	grpc "google.golang.org/grpc"
)

func RunPrivateGateway(ctx context.Context, app *app.App, r *mux.Router, conn *grpc.ClientConn, l net.Listener) error {

	gmux := runtime.NewServeMux(
		runtime.WithForwardResponseOption(gateway.CookieSetter(app.Config)), // Cookies always have to go first
		runtime.WithForwardResponseOption(gateway.StatusCodeMutator),
		// Workaround this limitation: https://github.com/grpc-ecosystem/grpc-gateway/issues/920.
		// Go's JSON encoder doesn't convert (u)int64 to strings silently.
		runtime.WithMarshalerOption(runtime.MIMEWildcard, gateway.JSONMarshaler()),
		runtime.WithMetadata(gateway.CookieAnnotator(app)),
	)

	public.RegisterPublicGatewayHandlers(ctx, app, r, gmux, conn)
	public.RegisterRoutes(r, app, gmux)

	err := authnpb.RegisterSecuredAdminAuthNHandler(ctx, gmux, conn)
	if err != nil {
		panic(err)
	}

	if app.Actives != nil {
		err := authnpb.RegisterAuthNActivesHandler(ctx, gmux, conn)
		if err != nil {
			panic(err)
		}
	}

	err = authnpb.RegisterUnsecuredAdminAuthNHandler(ctx, gmux, conn)
	if err != nil {
		panic(err)
	}

	RegisterRoutes(r, app, gmux)

	s := &http.Server{
		Addr:    l.Addr().String(),
		Handler: gateway.WrapRouter(gateway.FormWrapper(r), app),
	}

	go func() {
		<-ctx.Done()
		if err := s.Shutdown(context.Background()); err != nil {
			log.Printf("HTTP server Shutdown:: %v", err)
		}
	}()

	if err := s.Serve(l); err != nil && err != http.ErrServerClosed {
		return err
	}

	return nil
}
