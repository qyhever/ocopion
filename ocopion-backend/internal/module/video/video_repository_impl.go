package video

import (
	"path"
	"strings"
)

type VideoRepositoryImpl struct{}

func NewVideoRepository() Repository {
	return &VideoRepositoryImpl{}
}

func (r *VideoRepositoryImpl) GetVideoList() ([]VideoConfig, error) {
	list := []VideoConfig{
		{FileName: "24u7qivyunz.mp4", VideoName: "爸爸带着女儿买烧鸡"},
		{FileName: "20260628/char_clip4.mp4", VideoName: "单韵母"},
		{FileName: "20260628/char.mp4", VideoName: "一年级拼音"},
		{FileName: "a9d922da.mp4", VideoName: "太有才了这哥们"},
		{FileName: "a9d922dc.mp4", VideoName: "不要学电影里炒菜，都是假的"},
		{FileName: "last-night.mp4", VideoName: "英雄联盟CG-最后的曙光"},
		{FileName: "a9d922e1.mp4", VideoName: "舔着舔着突然发现不对劲"},
		{FileName: "vjsab8nx29p.mp4", VideoName: "今晚王者荣耀双排"},

		{FileName: "20260613/c4f3cea3de5cb1.mp4", VideoName: "猴子的无语瞬间"},
		{FileName: "20260613/c4f3cea3de5cb2.mp4", VideoName: "猴子吃魔鬼辣椒"},
		{FileName: "20260613/c4f3cea3de5cb3.mp4", VideoName: "猴子欺负游客"},
		{FileName: "20260613/c4f3cea3de5cb4.mp4", VideoName: "来吞"},
		{FileName: "20260613/c4f3cea3de5cb5.mp4", VideoName: "薯片桶整蛊猴子"},
		{FileName: "20260613/c4f3cea3de5cb6.mp4", VideoName: "动物们的无语瞬间"},

		{FileName: "20260530/20260530-a07fff81c4571.mp4", VideoName: "人类跳高精华大赏"},
		{FileName: "20260530/20260530-206cc334489d5.mp4", VideoName: "KTV小遛一首无地自容"},
		{FileName: "20260530/20260530-7d648e5917ce88.mp4", VideoName: "喆学家同事在KTV，让所有人都无地自容"},
		{FileName: "20260530/20260530-879cc2564a4a78.mp4", VideoName: "陶喆现在有些梗我都不懂了再现“哦哦啊诶”经典"},
		{FileName: "20260530/20260530-273fe3e6115568.mp4", VideoName: "陶喆婚礼太高兴怒飙生涯最高音G6！《今天你要嫁给我》"},
		{FileName: "20260530/20260530-2370e4f863a74.mp4", VideoName: "林志炫贴脸开大模仿陶喆名场面《胡彦斌你让我哭》"},
		{FileName: "20260530/20260530-ceafb5c93d229.mp4", VideoName: "来KTV挑战一下无地自容"},
		{FileName: "20260530/20260530-ceafb5c93d230.mp4", VideoName: "来KTV复刻无地自容"},
		{FileName: "20260530/20260530-ceafb5c93d231.mp4", VideoName: "南宁的朋友"},
		{FileName: "20260530/20260530-ceafb5c93d232.mp4", VideoName: "大爷在湖南景区喝水，猴子上前欲抢包"},
		{FileName: "20260530/20260530-ceafb5c93d233.mp4", VideoName: "无地自容防空警报"},
		{FileName: "20260530/20260530-ceafb5c93d234.mp4", VideoName: "无地自容小包包了包"},

		{FileName: "a9d9233b.mp4", VideoName: "351国道荆州松滋段"},
		{FileName: "a9d9233f.mp4", VideoName: "美洲豹vs凯门鳄"},
		{FileName: "a9d92409.mp4", VideoName: "无骨草鱼杀法 草鱼去骨去刺整个过程。"},
		{FileName: "acsgldq4ygzl.mp4", VideoName: "龙虾锁屏电脑"},
		{FileName: "d4eqp6p7vmm.mp4", VideoName: "臭豆腐超过印度美食"},
		{FileName: "i8ecwboeoci.mp4", VideoName: "这猝不及防的推背感"},
		{FileName: "j7kaeb6gmk.mp4", VideoName: "自己种的蒜苗"},
		{FileName: "1628846678209mp4.mp4", VideoName: "印度街头版汉堡这也太独特了"},
		{FileName: "1654937296227mp4.mp4", VideoName: "印度街头炒面，一斤面硬生生炒成了六两！"},
		{FileName: "2.mp4", VideoName: "lol觉醒CG动画，艾欧里亚vs偌克萨斯"},
		{FileName: "2013cg.mp4", VideoName: "2013年《英雄联盟》CG宣传片「扭曲的命运」AI修复补帧画质增强版"},
		{FileName: "2hzcohudabj.mp4", VideoName: "好久没玩了"},
		{FileName: "SuratFamousEggRecipe_TikhariOmelette_PintuBhai.mp4", VideoName: "你永远不知道印度的鸡蛋到哪一步才可以吃了"},
		{FileName: "nubg65mfcb.mp4", VideoName: "这里是公安县藕池口，为荆江南岸四口之一"},
		{FileName: "studio_video_1702118321696mp4.mp4", VideoName: "艾欧尼亚与诺克萨斯最为惨烈的一场战役"},
		{FileName: "v28dddg7yvr.mp4", VideoName: "黄豆应该是最全面的食物了，让我们来看看都可以做那些食物出来"},
		{FileName: "ios/IMG_5287.mp4", VideoName: "ios/IMG_5287.mp4"},
		{FileName: "ios/IMG_5288.mp4", VideoName: "ios/IMG_5288.mp4"},
		{FileName: "ios/IMG_5316.mp4", VideoName: "ios/IMG_5316.mp4"},
		{FileName: "ios/IMG_5324.mp4", VideoName: "ios/IMG_5324.mp4"},
		{FileName: "ios/IMG_5343.mp4", VideoName: "ios/IMG_5343.mp4"},
		{FileName: "ios/IMG_5347.mp4", VideoName: "ios/IMG_5347.mp4"},
		{FileName: "ios/IMG_5349.mp4", VideoName: "ios/IMG_5349.mp4"},
		{FileName: "ios/IMG_5354.mp4", VideoName: "ios/IMG_5354.mp4"},
		{FileName: "ios/IMG_5355.mp4", VideoName: "ios/IMG_5355.mp4"},
		{FileName: "ios/IMG_5359.mp4", VideoName: "ios/IMG_5359.mp4"},
		{FileName: "ios/IMG_5394.mp4", VideoName: "ios/IMG_5394.mp4"},
		{FileName: "ios/IMG_5397.mp4", VideoName: "ios/IMG_5397.mp4"},
		{FileName: "ios/IMG_5398.mp4", VideoName: "ios/IMG_5398.mp4"},
		{FileName: "ios/IMG_5399.mp4", VideoName: "ios/IMG_5399.mp4"},
		{FileName: "ios/IMG_5400.mp4", VideoName: "ios/IMG_5400.mp4"},
		{
			VideoName: "【P10】一年级下《静夜思》唐·李白.mp4",
			FileName:  "1007/a10870ec-1a5f-40e1-b0f0-f24b8eab10e1.mp4",
		},
		{
			VideoName: "【P11】一年级下《赠汪伦》唐·李白.mp4",
			FileName:  "1007/4f584caf-54a0-4e72-a820-540271e13db5.mp4",
		},
		{
			VideoName: "【P12】一年级下《画鸡》明·唐寅.mp4",
			FileName:  "1007/035f4f80-543c-4956-9f24-321f5423822e.mp4",
		},
		{
			VideoName: "【P13】一年级下《寻隐者不遇》唐·贾岛.mp4",
			FileName:  "1007/279cf1eb-b637-4bd2-aff0-a3815ef9f352.mp4",
		},
		{
			VideoName: "【P14】二年级上《敕勒歌》北朝民歌.mp4",
			FileName:  "1007/9afab5af-1313-492b-8659-48d9167864bd.mp4",
		},
		{
			VideoName: "【P15】二年级上《登鹳雀楼》唐·王之涣.mp4",
			FileName:  "1007/ecb28dae-6359-42db-b6c4-f66a1787d1be.mp4",
		},
		{
			VideoName: "【P16】二年级上《江雪》唐·柳宗元.mp4",
			FileName:  "1007/24a2c501-7df0-484e-9800-0e50869d5fb3.mp4",
		},
		{
			VideoName: "【P17】二年级上《梅花》宋·王安石.mp4",
			FileName:  "1007/a28ffeef-2069-4ad1-978f-51b5f413ae73.mp4",
		},
		{
			VideoName: "【P18】二年级上《望庐山瀑布》唐·李白.mp4",
			FileName:  "1007/7b6c00f4-e64a-4c90-8c4a-8b7c31a69a21.mp4",
		},
		{
			VideoName: "【P19】二年级上《小儿垂钓》唐·胡令能.mp4",
			FileName:  "1007/0f1956dc-8dbc-4796-95bb-392d3b549d66.mp4",
		},
		{
			VideoName: "【P1】一年级上《咏鹅》》唐·骆宾王.mp4",
			FileName:  "1007/aa5b144b-0b26-4861-b27d-f4777c1d7eaa.mp4",
		},
		{
			VideoName: "【P20】二年级上《夜宿山寺》唐·李白.mp4",
			FileName:  "1007/46d82fae-211f-4288-9d93-b3d9ccaaa7e7.mp4",
		},
		{
			VideoName: "【P21】2下（村居）清高鼎.mp4",
			FileName:  "1007/892f59ad-fb8c-4340-879b-10b183c95cf9.mp4",
		},
		{
			VideoName: "【P22】2下（赋得古原草送别）唐白居易.mp4",
			FileName:  "1007/d6871428-9f5f-476e-9b4a-cdd6349f5216.mp4",
		},
		{
			VideoName: "【P23】2下（画鸡）明唐寅.mp4",
			FileName:  "1007/08cd22db-f2eb-40e6-a3f5-75bd208782c8.mp4",
		},
		{
			VideoName: "【P24】2下（绝句）唐杜甫.mp4",
			FileName:  "1007/d9fae82b-e295-47e4-a4b3-b3049caa6289.mp4",
		},
		{
			VideoName: "【P25】2下（悯农其一）李绅.mp4",
			FileName:  "1007/6203370a-a6d2-4f3e-8735-bd5913b9764d.mp4",
		},
		{
			VideoName: "【P26】2下（晓出净慈寺送林子方）宋杨万里.mp4",
			FileName:  "1007/67b7a653-fec5-4dc1-87f2-1debf53c0464.mp4",
		},
		{
			VideoName: "【P27】2下（咏柳）唐贺知章.mp4",
			FileName:  "1007/4ca058e2-b4dc-48a7-8487-3f8f782c2467.mp4",
		},
		{
			VideoName: "【P28】2下（昼夜书所见）清查慎行.mp4",
			FileName:  "1007/004be400-b47b-4a93-a36a-a51d29832172.mp4",
		},
		{
			VideoName: "【P29】3上（山行）唐杜牧.mp4",
			FileName:  "1007/df7af047-2225-46a5-9ed1-e51a1a01d4ab.mp4",
		},
		{
			VideoName: "【P2】一年级上《古朗月行》唐·李白.mp4",
			FileName:  "1007/815f8f70-49b8-427d-a482-56b3a93fe317.mp4",
		},
		{
			VideoName: "【P30】3上（所见）清袁牧.mp4",
			FileName:  "1007/c1b19cc0-f9a4-4268-bf80-df5de240b0e8.mp4",
		},
		{
			VideoName: "【P31】3上《采莲曲》唐王昌龄.mp4",
			FileName:  "1007/9b051789-8e09-4382-8893-91209fa4cbed.mp4",
		},
		{
			VideoName: "【P32】3上《望洞庭》唐刘禹锡.mp4",
			FileName:  "1007/829bf6a2-680f-4f2b-aa78-5b4d16c7f211.mp4",
		},
		{
			VideoName: "【P33】3上《望天门山》唐李白.mp4",
			FileName:  "1007/02477b1d-a0d7-4da9-94cb-5ec8aa8c191c.mp4",
		},
		{
			VideoName: "【P34】3上《饮湖上初晴后雨》宋苏轼.mp4",
			FileName:  "1007/dc86cd7a-dd6a-4eb6-a2d5-9b693dd77542.mp4",
		},
		{
			VideoName: "【P35】3上《早发白帝城》唐李白.mp4",
			FileName:  "1007/0b644ca5-5e93-4b38-8692-1408a65c933f.mp4",
		},
		{
			VideoName: "【P36】3上《赠刘景文》宋苏轼.mp4",
			FileName:  "1007/cbe4d784-cefa-4df2-bc6e-1e359624edaa.mp4",
		},
		{
			VideoName: "【P37】3上《舟夜书所见》.mp4",
			FileName:  "1007/031b5fd5-648c-44bb-8029-3c7140b189fb.mp4",
		},
		{
			VideoName: "【P38】3下《滁州西涧》唐韦应物.mp4",
			FileName:  "1007/7ea33594-d1fd-4d94-87dd-dd64bee9ee36.mp4",
		},
		{
			VideoName: "【P39】3下《大林寺桃花》唐白居易.mp4",
			FileName:  "1007/a9bc3e60-e77c-43b1-902c-60ddc6ee94c9.mp4",
		},
		{
			VideoName: "【P3】一年级上《江南》汉乐府.mp4",
			FileName:  "1007/37007636-d16c-4033-bb67-ee7d4d86b678.mp4",
		},
		{
			VideoName: "【P40】3下《惠崇春江晚景》宋苏轼.mp4",
			FileName:  "1007/9d2bab91-0139-47a4-a10c-d0474b29f287.mp4",
		},
		{
			VideoName: "【P41】3下《九月九日忆山东兄弟》唐王维.mp4",
			FileName:  "1007/26a175ea-9d82-49e6-9adb-a6ca236c509b.mp4",
		},
		{
			VideoName: "【P42】3下《绝句》（迟日）唐杜甫.mp4",
			FileName:  "1007/2fe60df7-4166-4790-816a-fd597dddb38b.mp4",
		},
		{
			VideoName: "【P43】3下《清明》唐杜牧.mp4",
			FileName:  "1007/c2f02364-7add-485e-a5c7-45f2d99e4132.mp4",
		},
		{
			VideoName: "【P44】3下《三衢道中》宋曾几.mp4",
			FileName:  "1007/b0a05992-3cef-4fd6-9bee-c3299ae3ae8d.mp4",
		},
		{
			VideoName: "【P45】3下《忆江南》唐白居易.mp4",
			FileName:  "1007/08414ed2-d7f8-4ce8-8a1f-b4d9fc615e05.mp4",
		},
		{
			VideoName: "【P46】3下《元日》宋王安石.mp4",
			FileName:  "1007/7621fbaf-4c0f-4827-af3b-961c4beb34f4.mp4",
		},
		{
			VideoName: "【P4】一年级上《风》唐·李峤.mp4",
			FileName:  "1007/5ac5ff96-9d27-4ce9-838a-dce9302f24da.mp4",
		},
		{
			VideoName: "【P5】一年级上《悯农（其二）》李绅.mp4",
			FileName:  "1007/3d607f40-d86f-4723-8f2b-f18feccdf200.mp4",
		},
		{
			VideoName: "【P6】一年级上《画》唐·王维.mp4",
			FileName:  "1007/463bb4c9-ad77-42ca-ac70-07e16e8a51f4.mp4",
		},
		{
			VideoName: "【P7】一年级下《小池》宋·杨万里.mp4",
			FileName:  "1007/e25aa6d4-eff3-43c0-818c-1272cf15de00.mp4",
		},
		{
			VideoName: "【P8】一年级下《春晓》唐·孟浩然.mp4",
			FileName:  "1007/04bdb9ee-bc7a-479a-8530-428dc55c563e.mp4",
		},
		{
			VideoName: "【P9】一年级下《池上》唐·白居易.mp4",
			FileName:  "1007/e89eaec1-fc87-463b-ae9f-17fea22f25a3.mp4",
		},
	}

	for i := range list {
		list[i].Cover = videoCoverName(list[i].FileName)
	}

	return list, nil
}

func videoCoverName(fileName string) string {
	ext := path.Ext(fileName)
	if ext == "" {
		return fileName + ".png"
	}
	return strings.TrimSuffix(fileName, ext) + ".png"
}
