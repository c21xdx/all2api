package tabbit

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/google/uuid"
)

// RunLoginCommand prompts the user to log in manually and extract cookies.
func RunLoginCommand() (string, error) {
	loginURL := "https://web.tabbitbrowser.com/login"
	fmt.Printf("Please log in via the opened browser window.\nPort and Reverse proxy have been removed.\nIf it doesn't open automatically, visit:\n%s\n\n", loginURL)

	if err := openBrowser(loginURL); err != nil {
		fmt.Printf("Failed to open browser: %v\n", err)
	}

	fmt.Println("After logging in, please press F12, go to Application -> Storage -> Cookies.")

	jwtToken, err := readLongLine("token cookie")
	if err != nil {
		return "", fmt.Errorf("read token error: %w", err)
	}

	if jwtToken == "" {
		return "", fmt.Errorf("the token cannot be empty")
	}

	nextAuth, err := readLongLine("next-auth.session-token cookie")
	if err != nil {
		return "", fmt.Errorf("read next-auth.session-token error: %w", err)
	}

	deviceID := uuid.New().String()
	tokenParts := []string{jwtToken, nextAuth, deviceID}
	finalToken := strings.Join(tokenParts, "|")

	return finalToken, nil
}

func openBrowser(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start"}
	case "darwin":
		cmd = "open"
	default:
		cmd = "xdg-open"
	}
	args = append(args, url)
	return exec.Command(cmd, args...).Start()
}

// readLongLine handles extremely long JWT tokens by writing to a temporary file.
func readLongLine(prompt string) (string, error) {
	tmpFile, err := os.CreateTemp("", "tabbit-token-*.txt")
	if err != nil {
		return "", fmt.Errorf("无法创建临时文件: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	tmpPath := tmpFile.Name()
	tmpFile.Close() // close immediately so editor can use it

	fmt.Printf("\n>>> 你的终端可能无法直接粘贴超长文本。请在弹出的文本编辑器中粘贴您的 %s，保存并关闭编辑器 <<<\n", prompt)

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("notepad", tmpPath)
	case "darwin":
		cmd = exec.Command("open", "-a", "TextEdit", "-W", "-n", tmpPath) // -W waits for exit, -n opens new instance
	default:
		cmd = exec.Command("nano", tmpPath)
	}

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err = cmd.Run()
	if err != nil {
		fmt.Printf("请手动编辑此文件: %s\n", tmpPath)
		fmt.Print("编辑完成并保存后，按回车键继续... ")
		bufio.NewReader(os.Stdin).ReadBytes('\n')
	}

	data, err := os.ReadFile(tmpPath)
	if err != nil {
		return "", fmt.Errorf("读取 Token 文件失败: %w", err)
	}

	return strings.TrimSpace(string(data)), nil
}
