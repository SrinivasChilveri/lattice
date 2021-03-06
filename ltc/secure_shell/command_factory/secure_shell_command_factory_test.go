package command_factory_test

import (
	"errors"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"

	"github.com/cloudfoundry-incubator/lattice/ltc/app_examiner"
	"github.com/cloudfoundry-incubator/lattice/ltc/app_examiner/fake_app_examiner"
	config_package "github.com/cloudfoundry-incubator/lattice/ltc/config"
	"github.com/cloudfoundry-incubator/lattice/ltc/exit_handler/exit_codes"
	"github.com/cloudfoundry-incubator/lattice/ltc/exit_handler/fake_exit_handler"
	"github.com/cloudfoundry-incubator/lattice/ltc/secure_shell/command_factory"
	"github.com/cloudfoundry-incubator/lattice/ltc/secure_shell/command_factory/fake_secure_shell"
	"github.com/cloudfoundry-incubator/lattice/ltc/terminal"
	"github.com/cloudfoundry-incubator/lattice/ltc/test_helpers"
	"github.com/codegangsta/cli"
)

var _ = Describe("SSH CommandFactory", func() {
	var (
		config          *config_package.Config
		outputBuffer    *gbytes.Buffer
		terminalUI      terminal.UI
		fakeExitHandler *fake_exit_handler.FakeExitHandler
		fakeAppExaminer *fake_app_examiner.FakeAppExaminer
		fakeSecureShell *fake_secure_shell.FakeSecureShell
	)

	BeforeEach(func() {
		config = config_package.New(nil)
		config.SetTarget("lattice.xip.io")

		outputBuffer = gbytes.NewBuffer()
		terminalUI = terminal.NewUI(nil, outputBuffer, nil)
		fakeExitHandler = &fake_exit_handler.FakeExitHandler{}
		fakeAppExaminer = &fake_app_examiner.FakeAppExaminer{}
		fakeSecureShell = &fake_secure_shell.FakeSecureShell{}
	})

	Describe("SSHCommand", func() {
		var sshCommand cli.Command

		BeforeEach(func() {
			commandFactory := command_factory.NewSSHCommandFactory(config, terminalUI, fakeExitHandler, fakeAppExaminer, fakeSecureShell)
			sshCommand = commandFactory.MakeSSHCommand()
		})

		It("should ssh to instance 0 given an app name", func() {
			fakeAppExaminer.AppStatusReturns(app_examiner.AppInfo{ActualRunningInstances: 1}, nil)

			test_helpers.ExecuteCommandWithArgs(sshCommand, []string{"app-name"})

			Expect(outputBuffer).To(test_helpers.SayLine("Connecting to app-name/0 at %s", config.Target()))

			Expect(fakeSecureShell.ConnectToShellCallCount()).To(Equal(1))
			appName, instanceIndex, command, actualConfig := fakeSecureShell.ConnectToShellArgsForCall(0)
			Expect(appName).To(Equal("app-name"))
			Expect(instanceIndex).To(Equal(0))
			Expect(command).To(BeEmpty())
			Expect(actualConfig).To(Equal(config))

			Expect(fakeAppExaminer.AppStatusCallCount()).To(Equal(1))
			Expect(fakeAppExaminer.AppStatusArgsForCall(0)).To(Equal("app-name"))
		})

		It("should ssh to instance index specified", func() {
			fakeAppExaminer.AppStatusReturns(app_examiner.AppInfo{ActualRunningInstances: 3}, nil)

			test_helpers.ExecuteCommandWithArgs(sshCommand, []string{"--instance", "2", "app-name"})

			Expect(outputBuffer).To(test_helpers.SayLine("Connecting to app-name/2 at %s", config.Target()))

			Expect(fakeSecureShell.ConnectToShellCallCount()).To(Equal(1))
			appName, instanceIndex, command, actualConfig := fakeSecureShell.ConnectToShellArgsForCall(0)
			Expect(appName).To(Equal("app-name"))
			Expect(instanceIndex).To(Equal(2))
			Expect(command).To(BeEmpty())
			Expect(actualConfig).To(Equal(config))
		})

		It("should run a command remotely instead of the login shell", func() {
			fakeAppExaminer.AppStatusReturns(app_examiner.AppInfo{ActualRunningInstances: 1}, nil)

			doneChan := test_helpers.AsyncExecuteCommandWithArgs(sshCommand, []string{"app-name", "echo", "1", "2", "3"})

			Eventually(doneChan, 3).Should(BeClosed())
			Expect(outputBuffer).NotTo(test_helpers.Say("Connecting to app-name"))

			Expect(fakeSecureShell.ConnectToShellCallCount()).To(Equal(1))
			appName, instanceIndex, command, actualConfig := fakeSecureShell.ConnectToShellArgsForCall(0)
			Expect(appName).To(Equal("app-name"))
			Expect(instanceIndex).To(Equal(0))
			Expect(command).To(Equal("echo 1 2 3"))
			Expect(actualConfig).To(Equal(config))
		})

		It("should support -- delimiter for args", func() {
			fakeAppExaminer.AppStatusReturns(app_examiner.AppInfo{ActualRunningInstances: 1}, nil)

			doneChan := test_helpers.AsyncExecuteCommandWithArgs(sshCommand, []string{"app-name", "--", "/bin/ls", "-l"})

			Eventually(doneChan, 3).Should(BeClosed())
			Expect(outputBuffer).NotTo(test_helpers.Say("Connecting to app-name"))

			Expect(fakeSecureShell.ConnectToShellCallCount()).To(Equal(1))
			appName, instanceIndex, command, actualConfig := fakeSecureShell.ConnectToShellArgsForCall(0)
			Expect(appName).To(Equal("app-name"))
			Expect(instanceIndex).To(Equal(0))
			Expect(command).To(Equal("/bin/ls -l"))
			Expect(actualConfig).To(Equal(config))
		})

		Context("when not given an app name", func() {
			It("prints an error", func() {
				test_helpers.ExecuteCommandWithArgs(sshCommand, []string{})

				Expect(outputBuffer).To(test_helpers.SayIncorrectUsage())

				Expect(fakeSecureShell.ConnectToShellCallCount()).To(Equal(0))
				Expect(fakeExitHandler.ExitCalledWith).To(Equal([]int{exit_codes.InvalidSyntax}))
			})
		})

		Context("when given a non-existent app name", func() {
			It("prints an error", func() {
				fakeAppExaminer.AppStatusReturns(app_examiner.AppInfo{}, errors.New("no app"))

				test_helpers.ExecuteCommandWithArgs(sshCommand, []string{"bad-app"})

				Expect(outputBuffer).To(test_helpers.SayLine("App bad-app not found."))

				Expect(fakeSecureShell.ConnectToShellCallCount()).To(Equal(0))
				Expect(fakeExitHandler.ExitCalledWith).To(Equal([]int{exit_codes.CommandFailed}))
			})
		})

		Context("when given an invalid instance index", func() {
			It("prints an error", func() {
				fakeAppExaminer.AppStatusReturns(app_examiner.AppInfo{ActualRunningInstances: 1}, nil)

				test_helpers.ExecuteCommandWithArgs(sshCommand, []string{"good-app", "-i", "1"})

				Expect(outputBuffer).To(test_helpers.SayLine("Instance good-app/1 does not exist."))

				Expect(fakeSecureShell.ConnectToShellCallCount()).To(Equal(0))
				Expect(fakeExitHandler.ExitCalledWith).To(Equal([]int{exit_codes.CommandFailed}))
			})
		})

		Context("when ConnectToShell fails", func() {
			It("prints an error", func() {
				fakeAppExaminer.AppStatusReturns(app_examiner.AppInfo{ActualRunningInstances: 1}, nil)
				fakeSecureShell.ConnectToShellReturns(errors.New("connection failed"))

				test_helpers.ExecuteCommandWithArgs(sshCommand, []string{"good-app"})

				Expect(outputBuffer).To(test_helpers.SayLine("Error connecting to good-app/0: connection failed"))

				Expect(fakeSecureShell.ConnectToShellCallCount()).To(Equal(1))
				Expect(fakeExitHandler.ExitCalledWith).To(Equal([]int{exit_codes.CommandFailed}))
			})
		})
	})
})
