package main

import "github.com/cleverhu/longhorn-backup-restore/cmd"

func main() {
	command := cmd.NewLonghornBackupRestoreCommand()
	command.Execute()
}
