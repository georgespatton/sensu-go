package cmd

import (
	"fmt"
	"time"

	"github.com/AlecAivazis/survey"
	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/pkg/transport"
	corev2 "github.com/sensu/sensu-go/api/core/v2"
	"github.com/sensu/sensu-go/backend"
	"github.com/sensu/sensu-go/backend/etcd"
	"github.com/sensu/sensu-go/backend/seeds"
	etcdstore "github.com/sensu/sensu-go/backend/store/etcd"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	flagInitAdminUsername = "admin-username"
	flagInitAdminPassword = "admin-password"
	flagInteractive       = "interactive"
)

type seedConfig struct {
	backend.Config
	SeedConfig seeds.Config
}

type initOpts struct {
	AdminUsername string `survey:"admin-username"`
	AdminPassword string `survey:"admin-password"`
}

func (i *initOpts) administerQuestionnaire() error {
	qs := []*survey.Question{
		{
			Name: "admin-username",
			Prompt: &survey.Input{
				Message: "Admin Username:",
			},
			Validate: survey.Required,
		},
		{
			Name: "admin-password",
			Prompt: &survey.Password{
				Message: "Admin Password:",
			},
			Validate: survey.Required,
		},
	}

	return survey.Ask(qs, i)
}

// InitCommand is the 'sensu-backend init' subcommand.
func InitCommand() *cobra.Command {
	var setupErr error
	cmd := &cobra.Command{
		Use:           "init",
		Short:         "initialize a new sensu installation",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = viper.BindPFlags(cmd.Flags())
			if setupErr != nil {
				return setupErr
			}

			cfg := &backend.Config{
				EtcdAdvertiseClientURLs:      viper.GetStringSlice(flagEtcdAdvertiseClientURLs),
				EtcdListenClientURLs:         viper.GetStringSlice(flagEtcdClientURLs),
				EtcdListenPeerURLs:           viper.GetStringSlice(flagEtcdPeerURLs),
				EtcdInitialCluster:           viper.GetString(flagEtcdInitialCluster),
				EtcdInitialClusterState:      viper.GetString(flagEtcdInitialClusterState),
				EtcdInitialAdvertisePeerURLs: viper.GetStringSlice(flagEtcdInitialAdvertisePeerURLs),
				EtcdInitialClusterToken:      viper.GetString(flagEtcdInitialClusterToken),
				EtcdName:                     viper.GetString(flagEtcdNodeName),
				EtcdCipherSuites:             viper.GetStringSlice(flagEtcdCipherSuites),
				EtcdQuotaBackendBytes:        viper.GetInt64(flagEtcdQuotaBackendBytes),
				EtcdMaxRequestBytes:          viper.GetUint(flagEtcdMaxRequestBytes),
				EtcdHeartbeatInterval:        viper.GetUint(flagEtcdHeartbeatInterval),
				EtcdElectionTimeout:          viper.GetUint(flagEtcdElectionTimeout),
				NoEmbedEtcd:                  true,
			}

			// Sensu APIs TLS config
			certFile := viper.GetString(flagCertFile)
			keyFile := viper.GetString(flagKeyFile)
			insecureSkipTLSVerify := viper.GetBool(flagInsecureSkipTLSVerify)
			trustedCAFile := viper.GetString(flagTrustedCAFile)

			if certFile != "" && keyFile != "" {
				cfg.TLS = &corev2.TLSOptions{
					CertFile:           certFile,
					KeyFile:            keyFile,
					TrustedCAFile:      trustedCAFile,
					InsecureSkipVerify: insecureSkipTLSVerify,
				}
			} else if certFile != "" || keyFile != "" {
				return fmt.Errorf(
					"tls configuration error, both flags --%s & --%s are required",
					flagCertFile, flagKeyFile)
			}

			// Etcd TLS config
			cfg.EtcdClientTLSInfo = etcd.TLSInfo{
				CertFile:       viper.GetString(flagEtcdCertFile),
				KeyFile:        viper.GetString(flagEtcdKeyFile),
				TrustedCAFile:  viper.GetString(flagEtcdTrustedCAFile),
				ClientCertAuth: viper.GetBool(flagEtcdClientCertAuth),
			}

			// Convert the TLS config into etcd's transport.TLSInfo
			tlsInfo := (transport.TLSInfo)(cfg.EtcdClientTLSInfo)
			tlsConfig, err := tlsInfo.ClientConfig()
			if err != nil {
				return err
			}

			client, err := clientv3.New(clientv3.Config{
				Endpoints:   cfg.EtcdAdvertiseClientURLs,
				DialTimeout: 5 * time.Second,
				TLS:         tlsConfig,
			})

			if err != nil {
				return fmt.Errorf("error connecting to cluster: %s", err)
			}

			uname := viper.GetString(flagInitAdminUsername)
			pword := viper.GetString(flagInitAdminPassword)

			if viper.GetBool(flagInteractive) {
				var opts initOpts
				if err := opts.administerQuestionnaire(); err != nil {
					return err
				}
				uname = opts.AdminUsername
				pword = opts.AdminPassword
			}

			if uname == "" || pword == "" {
				return fmt.Errorf("both %s and %s are required to be set", flagInitAdminUsername, flagInitAdminPassword)
			}

			seedConfig := seedConfig{
				Config: *cfg,
				SeedConfig: seeds.Config{
					AdminUsername: uname,
					AdminPassword: pword,
				},
			}

			return seedCluster(client, seedConfig)
		},
	}

	cmd.Flags().String(flagInitAdminUsername, "", "cluster admin username")
	cmd.Flags().String(flagInitAdminPassword, "", "cluster admin password")
	cmd.Flags().Bool(flagInteractive, false, "interactive mode")

	setupErr = handleConfig(cmd)

	return cmd
}

func seedCluster(client *clientv3.Client, config seedConfig) error {
	store := etcdstore.NewStore(client, config.EtcdName)
	if err := seeds.SeedCluster(store, config.SeedConfig); err != nil {
		return fmt.Errorf("error initializing cluster: %s", err)
	}
	return nil
}
