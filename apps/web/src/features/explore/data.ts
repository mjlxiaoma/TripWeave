// 探索页的精选内容：热门目的地 + 行程模板。
// 模板 prompt 是给 AI 的自然语言描述（保持中文，生成效果最好），
// 一键创建旅行后由 planner autostart 接管生成。

export interface Destination {
  name: string
  nameEn: string
  emoji: string
  tagline: string
}

export interface TripTemplate {
  id: string
  title: string
  destination: string
  days: number
  emoji: string
  tags: string[]
  // 给 AI 的自然语言，用于一键生成行程
  prompt: string
}

export const DESTINATIONS: Destination[] = [
  { name: '北京', nameEn: 'Beijing', emoji: '🏛️', tagline: '皇城根下的历史' },
  { name: '上海', nameEn: 'Shanghai', emoji: '🌃', tagline: '摩登与烟火气' },
  { name: '成都', nameEn: 'Chengdu', emoji: '🐼', tagline: '美食与慢生活' },
  { name: '西安', nameEn: "Xi'an", emoji: '🏺', tagline: '十三朝古都' },
  { name: '杭州', nameEn: 'Hangzhou', emoji: '🌊', tagline: '西湖诗情画意' },
  { name: '大理', nameEn: 'Dali', emoji: '⛰️', tagline: '苍山洱海' },
  { name: '川西', nameEn: 'West Sichuan', emoji: '🏔️', tagline: '雪山与海子' },
  { name: '厦门', nameEn: 'Xiamen', emoji: '🌴', tagline: '海岛小清新' },
]

export const TEMPLATES: TripTemplate[] = [
  {
    id: 'beijing-classic',
    title: '北京经典 3 日',
    destination: '北京',
    days: 3,
    emoji: '🏛️',
    tags: ['历史文化', '经典必去'],
    prompt:
      '我想在北京玩3天，经典路线：第一天看升旗、逛故宫、景山俯瞰；第二天八达岭长城；第三天颐和园、圆明园，晚上后海或南锣鼓巷。节奏适中，要有当地美食。',
  },
  {
    id: 'chuanxi-loop',
    title: '川西小环线 4 日自驾',
    destination: '成都',
    days: 4,
    emoji: '🏔️',
    tags: ['自驾', '雪山', '自然风光'],
    prompt:
      '我想从成都出发自驾川西小环线4天，每天开车不超过4小时：Day1 经卧龙大熊猫基地、翻巴朗山到四姑娘山；Day2 双桥沟+猫鼻梁观景台；Day3 经小金河谷到丹巴看甲居藏寨；Day4 经泸定、雅安返回成都。',
  },
  {
    id: 'chengdu-foodie',
    title: '成都休闲 3 日',
    destination: '成都',
    days: 3,
    emoji: '🐼',
    tags: ['美食', '慢生活'],
    prompt:
      '我想在成都悠闲玩3天：看大熊猫、逛宽窄巷子和锦里、泡茶馆、吃火锅串串、去人民公园掏耳朵。节奏放慢，每天不要排太满，以吃为主。',
  },
  {
    id: 'yunnan-slow',
    title: '云南大理丽江 5 日',
    destination: '大理',
    days: 5,
    emoji: '⛰️',
    tags: ['治愈系', '古镇', '湖景'],
    prompt:
      '我想去云南大理丽江5天慢游：大理古城、洱海骑行、喜洲古镇，然后去丽江古城、束河古镇，时间够的话上苍山或去玉龙雪山。喜欢慢节奏，要有咖啡馆和发呆的时间。',
  },
  {
    id: 'hangzhou-weekend',
    title: '杭州周末 2 日',
    destination: '杭州',
    days: 2,
    emoji: '🌊',
    tags: ['周末', '短途', '湖景'],
    prompt:
      '我想利用周末在杭州玩2天：西湖环湖（断桥、苏堤、雷峰塔）、灵隐寺、龙井村喝茶，晚上河坊街或西湖边散步。轻松不累。',
  },
  {
    id: 'xian-history',
    title: '西安古都 3 日',
    destination: '西安',
    days: 3,
    emoji: '🏺',
    tags: ['历史文化', '博物馆'],
    prompt:
      '我想在西安玩3天看历史：兵马俑、陕西历史博物馆、大雁塔、城墙骑行、回民街吃肉夹馍和凉皮。喜欢博物馆和古迹，要有讲解价值的安排。',
  },
]
