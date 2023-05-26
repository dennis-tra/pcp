package receive

//type PeerState uint8
//
//const (
//	NotConnected PeerState = iota
//	Connecting
//	Connected
//	Authenticated
//	FailedConnecting
//	FailedAuthentication
//)
//
//type Node struct {
//	*pcpnode.Node
//
//	// mDNS discovery implementations
//	mdnsDiscoverer       *mdns.Discoverer
//	mdnsDiscovererOffset *mdns.Discoverer
//
//	// DHT discovery implementations
//	dhtDiscoverer       *dht.Discoverer
//	dhtDiscovererOffset *dht.Discoverer
//
//	autoAccept bool
//	peerStates sync.Map
//
//	// a logging service which updates the terminal with the current state
//	statusLogger *statusLogger
//
//	netNotifeeLk sync.RWMutex
//	netNotifee   *network.NotifyBundle
//}
//
//func InitNode(c *cli.Context, words []string) (*Node, error) {
//	h, err := pcpnode.New(c, words)
//	if err != nil {
//		return nil, err
//	}
//
//	node := &Node{
//		Node:       h,
//		autoAccept: c.Bool("auto-accept"),
//		peerStates: sync.Map{},
//	}
//
//	node.mdnsDiscoverer = mdns.NewDiscoverer(node, node)
//	node.mdnsDiscovererOffset = mdns.NewDiscoverer(node, node).SetOffset(-discovery.TruncateDuration)
//	node.dhtDiscoverer = dht.NewDiscoverer(node, node.DHT, node)
//	node.dhtDiscovererOffset = dht.NewDiscoverer(node, node.DHT, node).SetOffset(-discovery.TruncateDuration)
//	node.statusLogger = newStatusLogger(node)
//
//	node.RegisterPushRequestHandler(node)
//
//	// start logging the current status to the console
//	if !c.Bool("debug") {
//		go node.statusLogger.startLogging()
//	}
//
//	// stop the process if all discoverers error out
//	go node.watchDiscoverErrors()
//
//	// listen for shutdown event
//	go node.listenSigShutdown()
//
//	return node, nil
//}
//
//func (n *Node) listenSigShutdown() {
//	<-n.SigShutdown()
//	log.Debugln("Starting receive node shutdown procedurre")
//	n.stopDiscovering()
//	n.UnregisterTransferHandler()
//	n.statusLogger.Shutdown()
//
//	hostClosedChan := make(chan struct{})
//	go func() {
//		// TODO: properly closing the host can take up to 1 minute
//		if err := n.Host.Close(); err != nil {
//			log.Warningln("error stopping libp2p node:", err)
//		}
//		close(hostClosedChan)
//	}()
//
//	select {
//	case <-hostClosedChan:
//	case <-time.After(500 * time.Millisecond):
//	}
//
//	n.ServiceStopped()
//}
//
//func (n *Node) StartDiscoveringMDNS() {
//	n.SetState(pcpnode.Roaming)
//	go n.mdnsDiscoverer.Discover(n.ChanID)
//	go n.mdnsDiscovererOffset.Discover(n.ChanID)
//}
//
//func (n *Node) StartDiscoveringDHT() {
//	n.SetState(pcpnode.Roaming)
//	go n.dhtDiscoverer.Discover(n.ChanID)
//	go n.dhtDiscovererOffset.Discover(n.ChanID)
//}
//
//func (n *Node) stopDiscovering() {
//	n.mdnsDiscoverer.Shutdown()
//	n.mdnsDiscovererOffset.Shutdown()
//	n.dhtDiscoverer.Shutdown()
//	n.dhtDiscovererOffset.Shutdown()
//}
//
//func (n *Node) watchDiscoverErrors() {
//	for {
//		select {
//		case <-n.SigShutdown():
//			return
//		case <-n.mdnsDiscoverer.SigDone():
//		case <-n.mdnsDiscovererOffset.SigDone():
//		case <-n.dhtDiscoverer.SigDone():
//		case <-n.dhtDiscovererOffset.SigDone():
//		}
//		mdnsState := n.mdnsDiscoverer.State()
//		mdnsOffsetState := n.mdnsDiscovererOffset.State()
//		dhtState := n.dhtDiscoverer.State()
//		dhtOffsetState := n.dhtDiscovererOffset.State()
//
//		// if all discoverers errored out, stop the process
//		if mdnsState.Stage == mdns.StageError && mdnsOffsetState.Stage == mdns.StageError &&
//			dhtState.Stage == dht.StageError && dhtOffsetState.Stage == dht.StageError {
//			n.Shutdown()
//			return
//		}
//
//		// if all discoverers reached a termination stage (e.g., both were stopped or one was stopped, the other
//		// experienced an error), we have found and successfully connected to a peer. This means, all good - just
//		// stop this go routine.
//		if mdnsState.Stage.IsTermination() && mdnsOffsetState.Stage.IsTermination() &&
//			dhtState.Stage.IsTerminated() && dhtOffsetState.Stage.IsTerminated() {
//			log.Debugln("Stop watching discoverer errors")
//			return
//		}
//	}
//}
//
//// HandlePeerFound is called async from the discoverers. It's okay to have long-running tasks here.
//func (n *Node) HandlePeerFound(pi peer.AddrInfo) {
//	if n.GetState() != pcpnode.Roaming {
//		log.Debugln("Received a peer from the discoverer although we're not discovering")
//		return
//	}
//
//	// Add discovered peer to the hole punch allow list to track the
//	// hole punch state of that particular peer as soon as we try to
//	// connect to them.
//	n.AddToHolePunchAllowList(pi.ID)
//
//	// Check if we have already seen the peer and exit early to not connect again.
//	peerState, loaded := n.peerStates.LoadOrStore(pi.ID, Connecting)
//	if loaded {
//		switch peerState.(PeerState) {
//		case NotConnected:
//		case Connected:
//			log.Debugln("Ignoring discovered peer because as we're already connected", pi.ID)
//			return
//		case Connecting:
//			log.Debugln("Ignoring discovered peer as we're already trying to connect", pi.ID)
//			return
//		case Authenticated:
//			log.Debugln("Ignoring discovered peer as it's already authenticated", pi.ID)
//			return
//		case FailedConnecting:
//			// TODO: Check if multiaddrs have changed and only connect if that's the case
//			log.Debugln("We tried to connect previously but couldn't establish a connection, try again", pi.ID)
//		case FailedAuthentication:
//			log.Debugln("We tried to connect previously but the node didn't pass authentication -> skipping", pi.ID)
//			return
//		}
//	}
//
//	log.Debugln("Connecting to peer:", pi.ID)
//	if err := n.Connect(n.ServiceContext(), pi); err != nil {
//		log.Debugln("Error connecting to peer:", pi.ID, err)
//		n.peerStates.Store(pi.ID, FailedConnecting)
//		return
//	}
//	n.peerStates.Store(pi.ID, Connected)
//
//	n.DebugLogAuthenticatedPeer(pi.ID)
//
//	// lock push request handler until this handler has finished
//	// sometimes the push request comes too fast which messes up
//	// the log output. So we lock the other handler until
//	// all functions from this HandlePeerFound handler have finished.
//	n.LockHandler(pi.ID)
//	defer n.UnlockHandler(pi.ID)
//
//	// Negotiate PAKE
//	if _, err := n.StartKeyExchange(n.ServiceContext(), pi.ID); err != nil {
//		log.Errorln("Peer didn't pass authentication:", err)
//		if err == io.EOF {
//			n.peerStates.Store(pi.ID, FailedConnecting)
//		} else {
//			n.peerStates.Store(pi.ID, FailedAuthentication)
//		}
//		return
//	}
//	n.peerStates.Store(pi.ID, Authenticated)
//
//	// We're authenticated so can initiate a transfer
//	if n.SetState(pcpnode.Connected) == pcpnode.Connected {
//		log.Debugln("already connected and authenticated with another node")
//		return
//	}
//
//	// Stop the discovering process as we have found the valid peer
//	n.stopDiscovering()
//
//	// wait until the hole punch has succeeded
//	err := n.WaitForDirectConn(pi.ID)
//	if err != nil {
//		n.statusLogger.Shutdown()
//		n.Shutdown()
//		log.Infoln("Hole punching failed:", err)
//		return
//	}
//	n.statusLogger.Shutdown()
//
//	// make sure we don't open the new transfer-stream on the relayed connection.
//	// libp2p claims to not do that, but I have observed strange connection resets.
//	n.CloseRelayedConnections(pi.ID)
//}
//
//func (n *Node) SetConnected(pi peer.AddrInfo) {
//	n.SetState(pcpnode.Connected)
//
//	// Stop the discovering process as we have found the valid peer
//	n.stopDiscovering()
//
//	// wait until the hole punch has succeeded
//	err := n.WaitForDirectConn(pi.ID)
//	if err != nil {
//		n.statusLogger.Shutdown()
//		n.Shutdown()
//		log.Infoln("Hole punching failed:", err)
//		return
//	}
//	n.statusLogger.Shutdown()
//
//	// make sure we don't open the new transfer-stream on the relayed connection.
//	// libp2p claims to not do that, but I have observed strange connection resets.
//	n.CloseRelayedConnections(pi.ID)
//}
//
//func (n *Node) HandlePushRequest(pr *p2p.PushRequest) (bool, error) {
//	if n.autoAccept {
//		return n.handleAccept(pr)
//	}
//
//	obj := "File"
//	if pr.IsDir {
//		obj = "Directory"
//	}
//	log.Infof("%s: %s (%s)\n", obj, pr.Name, format.Bytes(pr.Size))
//	for {
//		log.Infof("Do you want to receive this %s? [y,n,i,?] ", strings.ToLower(obj))
//		scanner := bufio.NewScanner(os.Stdin)
//		if !scanner.Scan() {
//			return true, fmt.Errorf("failed reading from stdin: %w", scanner.Err())
//		}
//
//		// sanitize user input
//		input := strings.ToLower(strings.TrimSpace(scanner.Text()))
//
//		// Empty input, user just pressed enter => do nothing and prompt again
//		if input == "" {
//			continue
//		}
//
//		// Print the help text and prompt again
//		if input == "?" {
//			help()
//			continue
//		}
//
//		// Print information about the send request
//		if input == "i" {
//			printInformation(pr)
//			continue
//		}
//
//		// Accept the file transfer
//		if input == "y" {
//			return n.handleAccept(pr)
//		}
//
//		// Reject the file transfer
//		if input == "n" {
//			return false, nil
//		}
//
//		log.Infoln("Invalid input")
//	}
//}
//
//// handleAccept handles the case when the user accepted the transfer or provided
//// the corresponding command line flag.
//func (n *Node) handleAccept(pr *p2p.PushRequest) (bool, error) {
//	done := n.TransferFinishHandler(pr.Size)
//	th, err := NewTransferHandler(pr.Name, done)
//	if err != nil {
//		return true, err
//	}
//	n.RegisterTransferHandler(th)
//	return true, nil
//}
//
//func (n *Node) TransferFinishHandler(size int64) chan int64 {
//	done := make(chan int64)
//	go func() {
//		var received int64
//		select {
//		case <-n.SigShutdown():
//			return
//		case received = <-done:
//		}
//
//		if received == size {
//			log.Infoln("Successfully received file/directory!")
//		} else {
//			log.Infof(log.Yellow("WARNING: Only received %d of %d bytes!\n"), received, size)
//		}
//
//		n.Shutdown()
//	}()
//	return done
//}
