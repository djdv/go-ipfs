package fscmds

import (
	"context"
	"fmt"

	config "github.com/ipfs/go-ipfs-config"
	"github.com/ipfs/go-ipfs/filesystem"
	"github.com/ipfs/go-ipfs/filesystem/manager"
	"github.com/ipfs/go-ipfs/filesystem/manager/errors"
	"github.com/multiformats/go-multiaddr"
)

// TODO: too many words, it just splits and fills in empties, relaying unknowns, basically.
// move some of the section header documentation stuff
//
// fillFromConfig separates a request from its header,
// and compares sub-request input values, with corresponding settings in the config.
// (filling in missing sub-request value's if the config has one, and the request does not)
//
// Returns a stream of request headers, which map to a stream of sub-requests.
// If a request has a valid header that the config does not,
// the sub-request values are relayed unmodified;
// e.g. input `/namespace1/namespace2` results in `Request(nil)`
// when reading from `<-Header{namespace1;namespace2}.Requests`.
// (since the input request was valid, but contained no sub portion such as `/path/somewhere`)
func fillFromConfig(ctx context.Context,
	config *config.Config, requests manager.Requests) (sectionStream, errors.Stream) {

	relay, combinedErrors := make(chan section), make(chan errors.Stream, 1)
	sections, sectionErrors := splitRequests(ctx, requests)
	combinedErrors <- sectionErrors

	go func() {
		defer close(relay)
		defer close(combinedErrors)

		for section := range sections {
			switch section.API {
			// send sub-request value through a specific config section handler
			// mapping the sub-request stream, and merging the sub-error stream into ours
			case filesystem.Fuse:
				fuseRequests, fuseErrors := fillFuseConfig(ctx, config, section.ID, section.Requests)
				section.Requests = fuseRequests
				select {
				case combinedErrors <- fuseErrors:
				case <-ctx.Done():
					return
				}
			}

			select { // send the (potentially re-routed) section
			case relay <- section:
			case <-ctx.Done():
				return
			}
		}
	}()

	return relay, errors.Merge(ctx, combinedErrors)
}

// provides values for requests, from config
func fillFuseConfig(ctx context.Context, nodeConf *config.Config,
	nodeAPI filesystem.ID, requests manager.Requests) (manager.Requests, errors.Stream) {

	relay, errors := make(chan manager.Request), make(chan error)

	go func() {
		defer close(relay)
		defer close(errors)

		for request := range requests { // NOTE: request variable is re-used as the response value at end of loop
			var err error
			if request == nil { // request contains no (body) value (header only), use default value below
				err = multiaddr.ErrProtocolNotFound
			} else { //  request may contain the value we expect, check for it and handle error below
				_, err = multiaddr.Cast(request).ValueForProtocol(int(filesystem.PathProtocol))
			}

			switch err {
			case nil: // request has expected values, proceed
			case multiaddr.ErrProtocolNotFound: // request is missing a target value
				var requestMountpoint string
				switch nodeAPI { // supply one from the config's value
				case filesystem.IPFS:
					requestMountpoint = nodeConf.Mounts.IPFS
				case filesystem.IPNS:
					requestMountpoint = nodeConf.Mounts.IPNS
				default: // self explanatory
					err = fmt.Errorf("protocol %v has no config value", nodeAPI)
					goto respond // I'll argue about the label being on L:97 via goto vs L:77 via break
				}
				var configComponent *multiaddr.Component // marshal string -> request
				if configComponent, err = multiaddr.NewComponent(filesystem.PathProtocol.String(), requestMountpoint); err != nil {
					break
				}
				request = configComponent.Bytes() // output request is ready to be sent
			}

		respond:
			if err != nil {
				select {
				case <-ctx.Done():
				case errors <- err:
				}
				return
			}
			select {
			case relay <- request:
			case <-ctx.Done():
				return
			}
		}
	}()

	return relay, errors
}
