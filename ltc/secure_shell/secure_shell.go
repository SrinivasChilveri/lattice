package secure_shell

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"sync"
	"syscall"

	config_package "github.com/cloudfoundry-incubator/lattice/ltc/config"
	term_package "github.com/docker/docker/pkg/term"
	"golang.org/x/crypto/ssh"
)

//go:generate counterfeiter -o fake_dialer/fake_dialer.go . Dialer
type Dialer interface {
	Dial(user, authUser, authPassword, address string) (SecureSession, error)
}

//go:generate counterfeiter -o fake_secure_session/fake_secure_session.go . SecureSession
type SecureSession interface {
	StdinPipe() (io.WriteCloser, error)
	StdoutPipe() (io.Reader, error)
	StderrPipe() (io.Reader, error)
	SendRequest(name string, wantReply bool, payload []byte) (bool, error)
	RequestPty(term string, h, w int, termmodes ssh.TerminalModes) error
	Shell() error
	Run(string) error
	Wait() error
	Close() error
}

//go:generate counterfeiter -o fake_term/fake_term.go . Term
type Term interface {
	SetRawTerminal(fd uintptr) (*term_package.State, error)
	RestoreTerminal(fd uintptr, state *term_package.State) error
	GetWinsize(fd uintptr) (width int, height int)
}

type SecureShell struct {
	Dialer Dialer
	Term   Term
}

func (ss *SecureShell) ConnectToShell(appName string, instanceIndex int, command string, config *config_package.Config) error {
	diegoSSHUser := fmt.Sprintf("diego:%s/%d", appName, instanceIndex)
	address := fmt.Sprintf("%s:2222", config.Target())

	session, err := ss.Dialer.Dial(diegoSSHUser, config.Username(), config.Password(), address)
	if err != nil {
		return err
	}
	defer session.Close()

	sessionIn, err := session.StdinPipe()
	if err != nil {
		return err
	}

	sessionOut, err := session.StdoutPipe()
	if err != nil {
		return err
	}

	sessionErr, err := session.StderrPipe()
	if err != nil {
		return err
	}

	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 115200,
		ssh.TTY_OP_OSPEED: 115200,
	}

	width, height := ss.Term.GetWinsize(os.Stdout.Fd())

	terminalType := os.Getenv("TERM")
	if terminalType == "" {
		terminalType = "xterm"
	}

	if err := session.RequestPty(terminalType, height, width, modes); err != nil {
		return err
	}

	if state, err := ss.Term.SetRawTerminal(os.Stdin.Fd()); err == nil {
		defer ss.Term.RestoreTerminal(os.Stdin.Fd(), state)
	}

	go copyAndClose(nil, sessionIn, os.Stdin)
	go io.Copy(os.Stdout, sessionOut)
	go io.Copy(os.Stderr, sessionErr)

	resized := make(chan os.Signal, 16)
	signal.Notify(resized, syscall.SIGWINCH)
	defer func() {
		signal.Stop(resized)
		close(resized)
	}()
	go ss.resize(resized, session, os.Stdout.Fd(), width, height)

	if command == "" {
		session.Shell()
		session.Wait()
	} else {
		session.Run(command)
	}

	return nil
}

func copyAndClose(wg *sync.WaitGroup, dest io.WriteCloser, src io.Reader) {
	io.Copy(dest, src)
	dest.Close()
	if wg != nil {
		wg.Done()
	}
}

func (ss *SecureShell) resize(resized <-chan os.Signal, session SecureSession, terminalFd uintptr, initialWidth, initialHeight int) {
	type resizeMessage struct {
		Width       uint32
		Height      uint32
		PixelWidth  uint32
		PixelHeight uint32
	}

	var previousWidth, previousHeight int
	previousWidth = initialWidth
	previousHeight = initialHeight

	for _ = range resized {
		width, height := ss.Term.GetWinsize(terminalFd)

		if width == previousWidth && height == previousHeight {
			continue
		}

		message := resizeMessage{
			Width:  uint32(width),
			Height: uint32(height),
		}

		session.SendRequest("window-change", false, ssh.Marshal(message))

		previousWidth = width
		previousHeight = height
	}
}
