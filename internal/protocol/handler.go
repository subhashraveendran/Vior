package protocol

// MessageHandler processes incoming WebSocket messages from a client session.
type MessageHandler interface {
	OnHello(session *Session, msg *HelloMessage) error
	OnInput(session *Session, msg *InputMessage) error
	OnResize(session *Session, msg *ResizeMessage) error
	OnBye(session *Session) error
	OnFileOffer(session *Session, msg *FileOfferMessage) error
	OnFileAccept(session *Session, msg *FileAcceptMessage) error
	OnFileReject(session *Session, msg *FileRejectMessage) error
	OnFileChunk(session *Session, msg *FileChunkMessage) error
	OnFileComplete(session *Session, msg *FileCompleteMessage) error
	OnDownloadAccept(session *Session, msg *DownloadAcceptMessage) error
	OnDownloadReject(session *Session, msg *DownloadRejectMessage) error
	OnDownloadComplete(session *Session, msg *DownloadCompleteMessage) error
}
