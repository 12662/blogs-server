package service

type ServiceGroup struct {
	EsService
	BaseService
	JwtService
	GaodeService
	UserService
	QQService
	ImageService
	ArticleService
	AIService
	RAGService
	CommentService
	AdvertisementService
	FriendLinkService
	FeedbackService
	WebsiteService
	WechatService
	HotSearchService
	CalendarService
	ConfigService
}

var ServiceGroupApp = new(ServiceGroup)
