package get

import (
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/cli-runtime/pkg/printers"

	kargoapi "github.com/akuity/kargo/api/v1alpha1"
	"github.com/akuity/kargo/internal/cli/config"
	"github.com/akuity/kargo/internal/cli/option"
)

func NewCommand(cfg config.CLIConfig, streams genericiooptions.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get TYPE [NAME ...]",
		Short: "Display one or many resources",
		Args:  option.NoArgs,
		Example: `
# List all projects
kargo get projects

# List all stages in the project
kargo get stages --project=my-project

# List all promotions for the given stage
kargo get promotions --project=my-project --stage=my-stage
`,
	}

	// Register subcommands.
	cmd.AddCommand(newGetFreightCommand(cfg, streams))
	cmd.AddCommand(newGetProjectsCommand(cfg, streams))
	cmd.AddCommand(newGetPromotionsCommand(cfg, streams))
	cmd.AddCommand(newGetStagesCommand(cfg, streams))
	cmd.AddCommand(newGetWarehousesCommand(cfg, streams))
	return cmd
}

func printObjects[T runtime.Object](
	objects []T,
	flags *genericclioptions.PrintFlags,
	streams genericiooptions.IOStreams,
) error {
	items := make([]runtime.RawExtension, len(objects))
	for i, obj := range objects {
		items[i] = runtime.RawExtension{Object: obj}
	}
	list := &metav1.List{
		TypeMeta: metav1.TypeMeta{
			APIVersion: metav1.Unversioned.String(),
			Kind:       "List",
		},
		Items: items,
	}

	if flags.OutputFlagSpecified != nil && flags.OutputFlagSpecified() {
		printer, err := flags.ToPrinter()
		if err != nil {
			return errors.Wrap(err, "new printer")
		}
		if len(list.Items) == 1 {
			return printer.PrintObj(list.Items[0].Object, streams.Out)
		}
	}

	var t T
	var printObj runtime.Object
	switch any(t).(type) {
	case *kargoapi.Freight:
		printObj = newFreightTable(list)
	case *kargoapi.Project:
		printObj = newProjectTable(list)
	case *kargoapi.Promotion:
		printObj = newPromotionTable(list)
	case *kargoapi.Stage:
		printObj = newStageTable(list)
	default:
		printObj = list
	}
	return printers.NewTablePrinter(printers.PrintOptions{}).PrintObj(printObj, streams.Out)
}
