package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"strings"

	"google.golang.org/grpc/credentials"

	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	log "github.com/sirupsen/logrus"
	"github.com/soheilhy/cmux"

	"net"

	"github.com/gorilla/mux"
	"github.com/keratin/authn-server/app"
	"github.com/keratin/authn-server/grpc/internal/errors"
	"github.com/keratin/authn-server/grpc/private"
	"github.com/keratin/authn-server/grpc/public"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
)

func init() {
	runtime.HTTPError = errors.CustomHTTPError
}

// RunPrivateService starts a gRPC server for the private API and accompanying gRPC-Gateway server
func RunPrivateService(ctx context.Context, app *app.App, grpcListener net.Listener, httpListener net.Listener) error {

	privateRouter := mux.NewRouter()

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return private.RunPrivateGRPC(ctx, app, grpcListener)
	})

	connCreds := grpc.WithInsecure()
	if app.Config.ClientCA != nil {
		tlsConfig := &tls.Config{
			Certificates:       []tls.Certificate{app.Config.Certificate},
			ClientCAs:          app.Config.ClientCA,
			ClientAuth:         tls.RequireAndVerifyClientCert,
			InsecureSkipVerify: app.Config.TLSSkipVerify,
		}
		connCreds = grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig))
	}

	privClientConn, err := grpc.DialContext(ctx, grpcListener.Addr().String(), connCreds)
	if err != nil {
		log.Fatal(err)
	}

	g.Go(func() error {
		return private.RunPrivateGateway(ctx, app, privateRouter, privClientConn, httpListener)
	})

	return g.Wait()
}

// RunPublicService starts a gRPC server for the public API and accompanying gRPC-Gateway server
func RunPublicService(ctx context.Context, app *app.App, grpcListener net.Listener, httpListener net.Listener) error {

	publicRouter := mux.NewRouter()

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return public.RunPublicGRPC(ctx, app, grpcListener)
	})

	connCreds := grpc.WithInsecure()
	if app.Config.ClientCA != nil {
		tlsConfig := &tls.Config{
			Certificates:       []tls.Certificate{app.Config.Certificate},
			ClientCAs:          app.Config.ClientCA,
			ClientAuth:         tls.RequireAndVerifyClientCert,
			InsecureSkipVerify: app.Config.TLSSkipVerify,
		}
		connCreds = grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig))
	}

	clientConn, err := grpc.DialContext(ctx, grpcListener.Addr().String(), connCreds)
	if err != nil {
		log.Fatal(err)
	}

	g.Go(func() error {
		return public.RunPublicGateway(ctx, app, publicRouter, clientConn, httpListener)
	})

	return g.Wait()
}

func Server(app *app.App) {

	if app.Config.PublicPort != 0 {
		tcpl, err := net.Listen("tcp", fmt.Sprintf(":%d", app.Config.PublicPort))
		if err != nil {
			log.Fatalf("failed to listen: %v", err)
		}

		tcpm := cmux.New(tcpl)
		grpcl := tcpm.MatchWithWriters(cmux.HTTP2MatchHeaderFieldSendSettings("content-type", "application/grpc"))
		httpl := tcpm.Match(cmux.HTTP1Fast())
		go func() {
			if err := tcpm.Serve(); !strings.Contains(err.Error(), "use of closed network connection") {
				panic(err)
			}
		}()

		go func() {
			log.Fatal(RunPublicService(context.Background(), app, grpcl, httpl))
		}()
	}

	tcpl, err := net.Listen("tcp", fmt.Sprintf(":%d", app.Config.ServerPort))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	tcpm := cmux.New(tcpl)
	grpcl := tcpm.MatchWithWriters(cmux.HTTP2MatchHeaderFieldSendSettings("content-type", "application/grpc"))
	httpNoTLSL := tcpm.Match(cmux.HTTP1Fast())
	go func() {
		if err := tcpm.Serve(); !strings.Contains(err.Error(), "use of closed network connection") {
			panic(err)
		}
	}()

	log.Fatal(RunPrivateService(context.Background(), app, grpcl, httpNoTLSL))
}
