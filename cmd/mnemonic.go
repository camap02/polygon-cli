/*
Copyright © 2022 Polygon <engineering@polygon.technology>

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Lesser General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU Lesser General Public License for more details.

You should have received a copy of the GNU Lesser General Public License
along with this program. If not, see <http://www.gnu.org/licenses/>.
*/
package cmd

import (
	"fmt"

	"github.com/maticnetwork/polygon-cli/hdwallet"
	"github.com/spf13/cobra"
)

var (
	inputWords *int
	inputLang  *string
)

// mnemonicCmd represents the mnemonic command
var mnemonicCmd = &cobra.Command{
	Use:   "mnemonic",
	Short: "Generate a bip39 mnemonic seed",
	Long: `This is a basic function to generate a random seed phrase.

If you're looking to generate a full HD wallet, you'll need to use
some of the other commands, this command is meant only for generating
the mnemonic phrase rather than a full set of wallets and addresses
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		mnemonic, err := hdwallet.NewMnemonic(*inputWords, *inputLang)
		if err != nil {
			return err
		}
		cmd.Println(mnemonic)
		return nil
	},
	PreRunE: func(cmd *cobra.Command, args []string) error {
		if *inputWords < 12 {
			return fmt.Errorf("The number of words in the mnemonic must be 12 or more. Given: %d", *inputWords)
		}
		if *inputWords > 24 {
			return fmt.Errorf("The number of words in the mnemonic must be 24 or less. Given: %d", *inputWords)
		}
		if *inputWords%3 != 0 {
			return fmt.Errorf("The number of words in the mnemonic must be a multiple of 3")
		}
		return nil

	},
}

func init() {
	rootCmd.AddCommand(mnemonicCmd)
	inputWords = mnemonicCmd.PersistentFlags().Int("words", 24, "The number of words to use in the mnemonic")
	inputLang = mnemonicCmd.PersistentFlags().String("language", "english", "Which language to use [ChineseSimplified, ChineseTraditional, Czech, English, French, Italian, Japanese, Korean, Spanish]")
	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// mnemonicCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// mnemonicCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}