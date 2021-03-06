package command

import (
	"context"
	"os"
	"path"

	"github.com/SeerUK/foldup/pkg/archive"
	"github.com/SeerUK/foldup/pkg/foldup"
	"github.com/SeerUK/foldup/pkg/scheduling"
	"github.com/SeerUK/foldup/pkg/storage"
	"github.com/SeerUK/foldup/pkg/xioutil"
	"github.com/eidolon/console"
	"github.com/eidolon/console/parameters"
)

// BackupFmt is the filename format for the created archives.
const BackupFmt = "backup-%s-%d"

// For testing
var (
	archiveDirsf = archive.Dirsf
	osOpen       = os.Open
	osRemove     = os.Remove
	scheduleFunc = scheduling.ScheduleFunc
)

// BackupCommand creates a command to trigger periodic backups.
func BackupCommand(factory foldup.Factory) *console.Command {
	var bucket string
	var dirname string
	var schedule string

	configure := func(def *console.Definition) {
		def.AddArgument(console.ArgumentDefinition{
			Value: parameters.NewStringValue(&dirname),
			Spec:  "DIRNAME",
			Desc:  "The directory to archive folders from",
		})

		def.AddOption(console.OptionDefinition{
			Value: parameters.NewStringValue(&bucket),
			Spec:  "-b, --bucket=BUCKET",
			Desc:  "A bucket name to store the backups in.",
		})

		def.AddOption(console.OptionDefinition{
			Value: parameters.NewStringValue(&schedule),
			Spec:  "-s, --schedule=SCHEDULE",
			Desc:  "A cron-like expression, for scheduling recurring backups",
		})
	}

	execute := func(input *console.Input, output *console.Output) error {
		gateway, err := factory.CreateGCSGateway(bucket)
		if err != nil {
			return err
		}

		if schedule != "" {
			done := make(chan int)

			// Schedule a backup that will be recurring.
			return scheduleFunc(done, schedule, func() error {
				return doBackup(dirname, gateway)
			})
		}

		// Run a one-off backup.
		return doBackup(dirname, gateway)
	}

	return &console.Command{
		Name:        "backup",
		Description: "Back up the given directory.",
		Configure:   configure,
		Execute:     execute,
	}
}

// doBackup perform performs the actual backup, whether on a schedule or not.
func doBackup(dirname string, gateway storage.Gateway) error {
	// Read the directory names in the given directory.
	dirs, err := xioutil.ReadDirsInDir(dirname, false)
	if err != nil {
		return err
	}

	// Create the relative paths to those directories, so other code can find them.
	relativePaths := []string{}
	for _, d := range dirs {
		relativePaths = append(relativePaths, path.Join(dirname, d.Name()))
	}

	// Begin archiving the directories that were found.
	archives, err := archiveDirsf(relativePaths, BackupFmt, archive.TarGz)
	if err != nil {
		return err
	}

	// Upload each of the created archives to the storage.
	for _, a := range archives {
		in, err := osOpen(a)
		if err != nil {
			return err
		}

		err = gateway.Store(context.Background(), a, in)
		if err != nil {
			return err
		}

		err = osRemove(a)
		if err != nil {
			return err
		}
	}

	return nil
}
