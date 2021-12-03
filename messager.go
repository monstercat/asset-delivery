package asset_delivery

type Subscription interface {
	Unsubscribe()
}

type Messager interface {
	Publisher
	Subscriber
	Close()
}

type Subscriber interface {
	Subscribe(subj string, h func([]byte)) (Subscription, error)
}

type Publisher interface {
	Publish(subj string, data []byte) error
}