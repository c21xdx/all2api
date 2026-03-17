package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/adaptor"

	"github.com/lhpqaq/all2api/internal/config"
	internalserver "github.com/lhpqaq/all2api/internal/server"
	tabbit_auth "github.com/lhpqaq/all2api/internal/upstream/tabbit"
	"github.com/lhpqaq/all2api/internal/upstream/zed"
)

func main() {
	var cfgPath string
	flag.StringVar(&cfgPath, "config", "config.yaml", "config file path")

	isLogin := false
	var loginTarget string

	if len(os.Args) > 1 && os.Args[1] == "login" {
		isLogin = true
		if len(os.Args) > 2 {
			loginTarget = os.Args[2]
			os.Args = append([]string{os.Args[0]}, os.Args[3:]...)
		} else {
			loginTarget = "zed" // fallback default
			os.Args = append([]string{os.Args[0]}, os.Args[2:]...)
		}
	}

	flag.Parse()

	if isLogin {
		handleLogin(cfgPath, loginTarget)
		return
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	srv, err := internalserver.New(cfg)
	if err != nil {
		log.Fatalf("init server: %v", err)
	}

	hz := server.New(
		server.WithHostPorts(cfg.Server.Addr),
		server.WithReadTimeout(cfg.Server.ReadTimeout.Duration),
		server.WithWriteTimeout(cfg.Server.WriteTimeout.Duration),
		server.WithIdleTimeout(cfg.Server.IdleTimeout.Duration),
	)
	hz.Any("/*path", adaptor.HertzHandler(srv.Router()))

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	log.Printf("all2api listening on %s", cfg.Server.Addr)

	errCh := make(chan error, 1)
	go func() {
		errCh <- hz.Run()
	}()

	select {
	case <-stop:
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = hz.Shutdown(ctx)
		_ = <-errCh
	case err := <-errCh:
		if err != nil {
			log.Fatalf("listen: %v", err)
		}
	}
}

func handleLogin(cfgPath string, target string) {
	fmt.Printf("Starting %s Login Process...\n", target)
	var loginToken string
	var err error

	if target == "tabbit" {
		loginToken, err = tabbit_auth.RunLoginCommand()
	} else {
		loginToken, err = zed.RunLoginCommand()
	}
	if err != nil {
		fmt.Printf("Login Failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("\n=== SUCCESS ===")

	err = updateConfig(cfgPath, target, loginToken)
	if err == nil {
		fmt.Printf("Successfully updated %s upstreams in %s\n", target, cfgPath)
	} else {
		fmt.Printf("Warning: Failed to auto-update config file: %v\n", err)
		fmt.Println("Please manually add the token to your upstreams.")
	}

	fmt.Println("\nYour auth token:")
	fmt.Println("\n---------------------------------------------------------")
	fmt.Println(loginToken)
	fmt.Println("---------------------------------------------------------")
}

func updateConfig(filePath string, target string, token string) error {
	b, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	var root yaml.Node
	if err := yaml.Unmarshal(b, &root); err != nil {
		return err
	}

	if root.Kind != yaml.DocumentNode || len(root.Content) == 0 {
		return fmt.Errorf("empty yaml")
	}

	body := root.Content[0]
	var upstreamsIdx = -1
	for i := 0; i < len(body.Content); i += 2 {
		if body.Content[i].Value == "upstreams" {
			upstreamsIdx = i + 1
			break
		}
	}

	if upstreamsIdx == -1 {
		body.Content = append(body.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "upstreams"},
			&yaml.Node{Kind: yaml.MappingNode},
		)
		upstreamsIdx = len(body.Content) - 1
	}

	upms := body.Content[upstreamsIdx]
	if upms.Kind != yaml.MappingNode {
		return fmt.Errorf("upstreams is not mapping")
	}

	var targetIdx = -1
	for i := 0; i < len(upms.Content); i += 2 {
		if upms.Content[i].Value == target {
			targetIdx = i + 1
			break
		}
	}

	if targetIdx == -1 {
		targetYaml := fmt.Sprintf(`
type: "%s"
auth:
  kind: "token"
  token: "%s"`, target, token)
		var targetNode yaml.Node
		yaml.Unmarshal([]byte(targetYaml), &targetNode)

		upms.Content = append(upms.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: target},
			targetNode.Content[0],
		)
	} else {
		targetNode := upms.Content[targetIdx]
		var authIdx = -1
		for i := 0; i < len(targetNode.Content); i += 2 {
			if targetNode.Content[i].Value == "auth" {
				authIdx = i + 1
				break
			}
		}
		if authIdx == -1 {
			authYaml := fmt.Sprintf(`
kind: "token"
token: "%s"`, token)
			var authNode yaml.Node
			yaml.Unmarshal([]byte(authYaml), &authNode)
			targetNode.Content = append(targetNode.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Value: "auth"},
				authNode.Content[0],
			)
		} else {
			authNode := targetNode.Content[authIdx]
			var tokenIdx = -1
			for i := 0; i < len(authNode.Content); i += 2 {
				if authNode.Content[i].Value == "token" {
					tokenIdx = i + 1
					break
				}
			}
			if tokenIdx == -1 {
				authNode.Content = append(authNode.Content,
					&yaml.Node{Kind: yaml.ScalarNode, Value: "token"},
					&yaml.Node{Kind: yaml.ScalarNode, Value: token},
				)
			} else {
				authNode.Content[tokenIdx].Value = token
			}
		}
	}

	var out bytes.Buffer
	enc := yaml.NewEncoder(&out)
	enc.SetIndent(2)
	if err := enc.Encode(&root); err != nil {
		return err
	}
	enc.Close()

	return os.WriteFile(filePath, out.Bytes(), 0644)
}
