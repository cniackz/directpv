// This file is part of MinIO DirectPV
// Copyright (c) 2021, 2022 MinIO, Inc.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	directpvtypes "github.com/minio/directpv/pkg/apis/directpv.min.io/types"
	"github.com/minio/directpv/pkg/client"
	"github.com/minio/directpv/pkg/consts"
	"github.com/minio/directpv/pkg/drive"
	"github.com/minio/directpv/pkg/utils"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var accessTierArg string

var setDrivesCmd = &cobra.Command{
	Use:     "drives [DRIVE ...]",
	Aliases: []string{"drive", "dr"},
	Short:   "Set drives.",
	Example: strings.ReplaceAll(
		`# Set access-tier to all drives
$ kubectl {PLUGIN_NAME} set drives --access-tier=hot

# Set access-tier to all drives from all nodes
$ kubectl {PLUGIN_NAME} set drives --all --access-tier=warm

# Set access-tier to all drives from a node
$ kubectl {PLUGIN_NAME} set drives --node=node1 --access-tier=cold

# Set access-tier to drives from all nodes
$ kubectl {PLUGIN_NAME} set drives --drive-name=sda --access-tier=hot

# Set access-tier to specific drives from specific nodes
$ kubectl {PLUGIN_NAME} set drives --node=node{1...4} --drive-name=sd{a...f} --access-tier=warm`,
		`{PLUGIN_NAME}`,
		consts.AppName,
	),
	Run: func(c *cobra.Command, args []string) {
		driveIDArgs = args

		if err := validateSetDrivesCmd(); err != nil {
			utils.Eprintf(quietFlag, true, "%v\n", err)
			os.Exit(-1)
		}

		accessTiers, err := directpvtypes.StringsToAccessTiers(accessTierArg)
		if err != nil {
			utils.Eprintf(quietFlag, true, "%v\n", err)
			os.Exit(-1)
		}

		setDrivesMain(c.Context(), accessTiers[0])
	},
}

func init() {
	addDriveStatusFlag(setDrivesCmd, "If present, select drives by status")
	addAllFlag(setDrivesCmd, "If present, select all drives")
	addDryRunFlag(setDrivesCmd)
	setDrivesCmd.PersistentFlags().StringVar(&accessTierArg, "access-tier", accessTierArg, fmt.Sprintf("Set access-tier; one of: %v", strings.Join(accessTierValues, "|")))
}

func validateSetDrivesCmd() error {
	if err := validateNodeArgs(); err != nil {
		return err
	}

	if err := validateDriveNameArgs(); err != nil {
		return err
	}

	if err := validateDriveStatusArgs(); err != nil {
		return err
	}

	if err := validateDriveIDArgs(); err != nil {
		return err
	}

	if accessTierArg == "" {
		return fmt.Errorf("--access-tier must be provided")
	}

	accessTierArg = strings.TrimSpace(accessTierArg)
	if !utils.Contains(accessTierValues, accessTierArg) {
		return fmt.Errorf("unknown access-tier %v", accessTierArg)
	}

	switch {
	case allFlag:
	case len(nodeArgs) != 0:
	case len(driveNameArgs) != 0:
	case len(driveStatusArgs) != 0:
	case len(driveIDArgs) != 0:
	default:
		return errors.New("no drive selected to set properties")
	}

	if allFlag {
		nodeArgs = nil
		driveNameArgs = nil
		driveStatusSelectors = nil
		driveIDSelectors = nil
	}

	return nil
}

func setDrivesMain(ctx context.Context, accessTier directpvtypes.AccessTier) {
	var processed bool

	ctx, cancelFunc := context.WithCancel(ctx)
	defer cancelFunc()

	resultCh := drive.NewLister().
		NodeSelector(toLabelValues(nodeArgs)).
		DriveNameSelector(toLabelValues(driveNameArgs)).
		AccessTierSelector(toLabelValues(accessTierArgs)).
		StatusSelector(driveStatusSelectors).
		DriveIDSelector(driveIDSelectors).
		List(ctx)
	for result := range resultCh {
		if result.Err != nil {
			utils.Eprintf(quietFlag, true, "%v\n", result.Err)
			os.Exit(1)
		}

		processed = true
		switch {
		case result.Drive.GetAccessTier() == accessTier:
			utils.Eprintf(quietFlag, false, "%v/%v already set\n", result.Drive.GetNodeID(), result.Drive.GetDriveName())
		default:
			result.Drive.SetAccessTier(accessTier)
			var err error
			if !dryRunFlag {
				_, err = client.DriveClient().Update(ctx, &result.Drive, metav1.UpdateOptions{})
			}
			if err != nil {
				utils.Eprintf(quietFlag, true, "%v/%v: %v\n", result.Drive.GetNodeID(), result.Drive.GetDriveName(), err)
			} else if !quietFlag {
				fmt.Printf("Processed %v/%v\n", result.Drive.GetNodeID(), result.Drive.GetDriveName())
			}
		}
	}

	if !processed {
		if allFlag {
			utils.Eprintf(quietFlag, false, "No resources found\n")
		} else {
			utils.Eprintf(quietFlag, false, "No matching resources found\n")
		}

		os.Exit(1)
	}
}
