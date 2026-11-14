package models

// The rest of the catalogue: Voxt's other local models (Voxt 1.15's
// MLXModelCatalog and CustomLLMModelCatalog) and the family's Gemma 4
// E4B, below the recommended ones. The speech models' pins are
// sglang_omni_mlx/native/ci/models.json's and model_pins.json's; the
// language models' are the family pins table's where it has the repo,
// else the repo's commit of 2026-11-12 (docs/models-page.md). Their files
// are those the runtimes load: weights, configs, tokenizer and chat
// template, not the OptiQ vision tower and MTP head, nor Mistral's
// native tekken.json and params.json.
//
// Only models whose cards claim English, Chinese and Japanese: VoxtMini
// speaks those three (user, 2026-11-13; docs/models-page.md, "Languages"
// says which models were dropped and why).

// The more models' ids.
const (
	ASRQwen6bit     = "asr.qwen3-1.7b-6bit"
	ASRWhisperTurbo = "asr.whisper-turbo"
	ASRWhisperLarge = "asr.whisper-large"
	ASRWhisperSmall = "asr.whisper-small"
	LLMGemma4       = "llm.gemma4"
	LLMQwen35_9B    = "llm.qwen35-9b"
	LLMGemma4_12B   = "llm.gemma4-12b"
	LLMQwen36_27B   = "llm.qwen36-27b"
	LLMMinistral3   = "llm.ministral3"
	LLMGemma4E2B    = "llm.gemma4-e2b"
	LLMLFM2_8B      = "llm.lfm2-8b"
	LLMLFM2         = "llm.lfm2"
)

var more = []Model{
	{
		ID: ASRQwen6bit, Kind: ASR, Runtime: Omni, Title: "Qwen3-ASR 1.7B · 6-bit", Experimental: true, Needs: []string{VADSilero},
		Family: "qwen3-asr", Quant: "6bit",
		Repo: "mlx-community/Qwen3-ASR-1.7B-6bit", Revision: "edd077a475c4da058e25b6e6ec1199115ca1be2b",
		Files: []File{
			{"chat_template.json", 1161, "75a8cfca24f00de72d796fbfed6858fc9614ef3dabd8696684cc3bc03a9c58ff", false},
			{"config.json", 7188, "243c07b95f6ade8db20e73b3aa2b0ba101f8bcce75b82557f7d8e957838bcf25", false},
			{"generation_config.json", 142, "1da527824d81e07118facff437e03f2e24a23311e3bdeb2368973fe77e5f275c", false},
			{"merges.txt", 1671853, "8831e4f1a044471340f7c0a83d7bd71306a5b867e95fd870f74d0c5308a904d5", false},
			{"model.safetensors", 2033194557, "cacb094fef227ec2a5908d0136b3ad3384d95e0e2a8a915984ce577e4b12b2ba", true},
			{"model.safetensors.index.json", 78968, "703625934380698dbe06a62aa0e50167ec30fb4d9a383da171af6dd07c56e82d", false},
			{"preprocessor_config.json", 330, "45e120a4eda2c20c5d7f2ea9354e63536bf35e27aa573fb7cdf78017b378770d", false},
			{"tokenizer_config.json", 12487, "4942d005604266809309cabc9f4e9cb89ce855d59b14681fdc0e1cc62ea26c4c", false},
			{"vocab.json", 2776833, "ca10d7e9fb3ed18575dd1e277a2579c16d108e32f27439684afa0e10b1440910", false},
		},
	},
	{
		ID: ASRWhisperTurbo, Kind: ASR, Runtime: Omni, Title: "Whisper large-v3-turbo", Experimental: true, Needs: []string{VADSilero},
		Family: "whisper", Quant: "fp16",
		Aux:  whisperLargeV3Tokenizer,
		Repo: "mlx-community/whisper-large-v3-turbo", Revision: "a4aaeec0636e6fef84abdcbe3544cb2bf7e9f6fb",
		Files: []File{
			{"config.json", 268, "b34fc29e4e11e0a25e812775dd67f4dd16fc2c8eb43d28ae25ff7d660ecb6379", false},
			{"weights.safetensors", 1613977612, "951ed3fc1203e6a62467abb2144a96ce7eafca8fa77e3704fdb8635ff3e7f8a6", true},
		},
	},
	{
		ID: ASRWhisperLarge, Kind: ASR, Runtime: Omni, Title: "Whisper large-v3", Experimental: true, Needs: []string{VADSilero},
		Family: "whisper", Quant: "fp16",
		Aux:  whisperLargeV3Tokenizer,
		Repo: "mlx-community/whisper-large-v3-mlx", Revision: "49e6aa286ad60c14352c404340ded53710378a11",
		Files: []File{
			{"config.json", 269, "34982ce6ae286095000f82ae9583b3431639e8b092bf60c961f203745e6500e3", false},
			{"weights.npz", 3083520416, "05ff791ce3630fae47e7c51004e9666204d786246ec07cac6110af768099b40d", true},
		},
	},
	{
		ID: ASRWhisperSmall, Kind: ASR, Runtime: Omni, Title: "Whisper small", Experimental: true, Needs: []string{VADSilero},
		Family: "whisper", Quant: "fp16",
		Aux:  whisperSmallTokenizer,
		Repo: "mlx-community/whisper-small-mlx", Revision: "45f3915923c7a79a5a5b5a7d909d39aeb0e5630e",
		Files: []File{
			{"config.json", 266, "e8f58e638208af66d5d5d67801259dc7a12d199e971967a9f9d33a8e3635668e", false},
			{"weights.npz", 481307592, "55b6674c9b339702d486e2b1573839a66f8ec8f821ed2886993ef717a86b09f5", true},
		},
	},
	{
		ID: LLMGemma4, Kind: LLM, Runtime: MLX, Title: "Gemma 4 E4B · 4-bit",
		Family: "gemma4", Quant: "4bit",
		Repo: "mlx-community/gemma-4-e4b-it-4bit", Revision: "475b9088d29754a3379866cf5aeb6b41acd313c2",
		Files: []File{
			{"chat_template.jinja", 17336, "2f1b4d75d067bae3fe44e676721c7f077d243bc007156cb9c2f8b5836613d082", false},
			{"config.json", 6628, "780ccb3a514a5f1ced161d383f948fc22eca9b84b752ca19494f625bd9bad7a6", false},
			{"generation_config.json", 208, "d4226bbe3117d2d253ba4609720ba82c6c4ce4627a9a6ae05387c78983ac03de", false},
			{"model.safetensors", 5146800534, "932b8271fc3fe65adcc78b96c10c6268bbfb13e8f67d1358727c0d6ee97e1eff", true},
			{"model.safetensors.index.json", 240961, "f8accac59ee7efe87e0c298c854610b262c3cadd477407503147c71209ff0093", false},
			{"processor_config.json", 1316, "de3e580aebdc98272d4c4547daffe6525fcbae18a83a0e0bcf0d7444d4ee6f37", false},
			{"tokenizer.json", 32169626, "cc8d3a0ce36466ccc1278bf987df5f71db1719b9ca6b4118264f45cb627bfe0f", true},
			{"tokenizer_config.json", 2740, "080d9e1aff284e2f6043889cd05367966f7c7b80e025fbc0b06745e218158656", false},
		},
	},
	{
		ID: LLMQwen35_9B, Kind: LLM, Runtime: MLX, Title: "Qwen3.5 9B OptiQ · 4-bit", Experimental: true,
		Family: "qwen3.5", Quant: "4bit",
		Repo: "mlx-community/Qwen3.5-9B-OptiQ-4bit", Revision: "10a7fc93870c02a14b6bd5ddef4a7f6ada5f111f",
		Files: []File{
			{"chat_template.jinja", 7756, "a4aee8afcf2e0711942cf848899be66016f8d14a889ff9ede07bca099c28f715", false},
			{"config.json", 55664, "7c1a6013d5355749280bcf9ad8b7c83fa44c0cc7657526cd742716b9348128c4", false},
			{"generation_config.json", 148, "f2130efdf66df1834aa4f7a6e48752e88a8dff41965739c6ffb2bb26c841615e", false},
			{"kv_config.json", 528, "88b7f80fd9b3a202088f71889a7b43f818d63c4f836a60f26d5babe08093a10c", false},
			{"model-00001-of-00002.safetensors", 5349382977, "ac85c6ecdcd12ab778d44a9d6f90f1080498173c7fa6da2f335f3c42b716d85c", true},
			{"model-00002-of-00002.safetensors", 1751328661, "9d3a23b2307c705af2e86bbf2af791f0d63eae8c24042c5f362ad7586caa0062", true},
			{"model.safetensors.index.json", 115912, "71a97da2f5c6b961b02d55f099b5e23dee6280dd3eece93acac30e02534e24a6", false},
			{"optiq_metadata.json", 26233, "ee34e7aec78859db7f3473e1b3c75170de0985b71aabd971aa6344c9f792dec8", false},
			{"preprocessor_config.json", 390, "27225450ac9c6529872ee1924fcb0962ff5634834f817040f444118116f4e516", false},
			{"processor_config.json", 1300, "14932921ca485d458a04dafd8069fbb0a4505622a48208d19ed247115801385b", false},
			{"tokenizer.json", 19989343, "87a7830d63fcf43bf241c3c5242e96e62dd3fdc29224ca26fed8ea333db72de4", true},
			{"tokenizer_config.json", 1135, "b2bed5e033438f09f22b0ce9522115b4807d9bbcc3b82bf23372cb74d93ed081", false},
			{"video_preprocessor_config.json", 385, "7768af27c1fafa9cc9011c1dc20067e03f8915e03b63504550e11d5066986d13", false},
		},
	},
	{
		ID: LLMGemma4_12B, Kind: LLM, Runtime: MLX, Title: "Gemma 4 12B OptiQ · 4-bit", Experimental: true,
		Family: "gemma4", Quant: "4bit",
		Repo: "mlx-community/gemma-4-12B-it-OptiQ-4bit", Revision: "de9e183868050b34bf6b4272518500ffcebf15b3",
		Files: []File{
			{"chat_template.jinja", 18683, "ae53464bf3be25802b3a5b37def7fd89667067d7577049b3b2d74c4d8de4c6d4", false},
			{"config.json", 71981, "a9eb82640fbc1f4c1bcc57d6d657f1f192b00f7187b1fdd8657a3e1568d703a0", false},
			{"generation_config.json", 260, "a8349d9bd64cc5841297fcb5002f0fdc4749c473c8f1b10ea337f9ce4ee7014e", false},
			{"kv_config.json", 3160, "da38d18d32a7d4ce86b0e678a6d7574f250c5d5a12c680cda176cc9677dc73a9", false},
			{"model-00001-of-00002.safetensors", 5349856755, "348558c33cc1910f36123b00476142c92a6e234d291e3b51e32be3924a4bd30f", true},
			{"model-00002-of-00002.safetensors", 3510027171, "df076d7ce133395f261902f4be5ff6104efacb2bcf02b8c74887501725cca1ec", true},
			{"model.safetensors.index.json", 129412, "6f58c5ed0103b261e0f3613faad44ee014dc81d3566a45f47b48eae1d7efbc46", false},
			{"optiq_metadata.json", 33780, "6c783eddeafede171db14512f220d0dc51ecb929f301f04745e018b1b7f613e0", false},
			{"processor_config.json", 868, "016a1db9c4f41ea0c61919c46855ea5e7c45c6e4ae4bfbedfb5b6bed79a2fe92", false},
			{"tokenizer.json", 32169626, "cc8d3a0ce36466ccc1278bf987df5f71db1719b9ca6b4118264f45cb627bfe0f", true},
			{"tokenizer_config.json", 2780, "113b1b1773839fe736d09a3c80a63a3877c0ae1321726f56c8bb6c7cb1943710", false},
		},
	},
	{
		ID: LLMQwen36_27B, Kind: LLM, Runtime: MLX, Title: "Qwen3.6 27B · 4-bit", Experimental: true,
		Family: "qwen3.6", Quant: "4bit",
		Repo: "mlx-community/Qwen3.6-27B-4bit", Revision: "c000ac2c2057d94be3fa931000c31723aac53282",
		Files: []File{
			{"chat_template.jinja", 7764, "e84f32a23fdda27689f868aa4a1a5621f41133e51a48d7f3efcbea2839574259", false},
			{"config.json", 4573, "ede24666ac51e6d5ab948a8a1e6c72fc6effd941ba3aabb6dd942eb517c78043", false},
			{"configuration.json", 51, "2d4464e2ead06bc9bc718c781309ad1e7baded626d66e8dcdc8b469ba185faf0", false},
			{"generation_config.json", 202, "e70c136c1b78ddc1fb0905bac8e733a4dc448d4f852a5dd75143fffc70be550e", false},
			{"model-00001-of-00003.safetensors", 5343268752, "2689680915661f040c50c35244d08b336def279e509e1ca11873f8dd1b0e7ce0", true},
			{"model-00002-of-00003.safetensors", 5354185100, "a46183727b2c5cd16613fd8395f5e9e7cc4ad679644558cc87c9406fe334463d", true},
			{"model-00003-of-00003.safetensors", 5357087747, "ac23bf70b1f239a040921d6f93770d74176fd435dbf44e42317053d06c68d702", true},
			{"model.safetensors.index.json", 218281, "13b840162b4cb35c66fef7df072f7dbb4717908204364f5e5d9f9655a2758fa8", false},
			{"preprocessor_config.json", 390, "27225450ac9c6529872ee1924fcb0962ff5634834f817040f444118116f4e516", false},
			{"processor_config.json", 1312, "51c5f33646392e0bf1ed3e96be02504da47406c21c6cb86925d8908d1a6c4da7", false},
			{"tokenizer.json", 19989343, "87a7830d63fcf43bf241c3c5242e96e62dd3fdc29224ca26fed8ea333db72de4", true},
			{"tokenizer_config.json", 1139, "e98f1901ac6f0adff67b1d540bfa0c36ac1a0cf59eb72ed78146ef89aafa1182", false},
			{"video_preprocessor_config.json", 385, "7768af27c1fafa9cc9011c1dc20067e03f8915e03b63504550e11d5066986d13", false},
			{"vocab.json", 6722759, "ce99b4cb2983d118806ce0a8b777a35b093e2000a503ebde25853284c9dfa003", false},
		},
	},
	{
		ID: LLMMinistral3, Kind: LLM, Runtime: MLX, Title: "Ministral 3 3B · 4-bit", Experimental: true,
		Family: "ministral3", Quant: "4bit",
		Repo: "mlx-community/Ministral-3-3B-Instruct-2512-4bit", Revision: "a962dcb09eee4169c890e544c9eb938f1113fdee",
		Files: []File{
			{"chat_template.jinja", 7759, "c101a09b4ff42e8ed714cb67cd7144176e169b7d7be45d62e1e13326a5d7afd7", false},
			{"config.json", 1988, "cdd4bfa0a702d0241faaa3b80d15d481d7f8abf2659fba0958b1e7f24a28055f", false},
			{"generation_config.json", 131, "e0923390059f84a9180b00e5501778acc45ea9856cd7f2fd68208b360927c677", false},
			{"model.safetensors", 2745210934, "69a68c0a961a64bb1145a697a83cf6cec45220d425c77a6c0a3568924078c3b4", true},
			{"model.safetensors.index.json", 74906, "22e571d413d8aa4e5f81f5ae678aa8b4f5b1d67598a67c08b31182075592ec93", false},
			{"processor_config.json", 976, "ece2373c2ae391bce18785a1810543bc6173a1f28b6767cffab2c35dbea5f002", false},
			{"tokenizer.json", 17077402, "286acad9b0e27fce778ac429763536accf618ccb6ed72963b6f94685e531c5c7", true},
			{"tokenizer_config.json", 21188, "54b6ee53caf87c338527472386b6adcc4253bf48b93cc9c27fe1d25db4ff36fc", false},
		},
	},
	{
		ID: LLMGemma4E2B, Kind: LLM, Runtime: MLX, Title: "Gemma 4 E2B · 4-bit", Experimental: true,
		Family: "gemma4", Quant: "4bit",
		Repo: "mlx-community/gemma-4-e2b-it-4bit", Revision: "238767527555cb75a05732a84dff5d6ba0dd6809",
		Files: []File{
			{"chat_template.jinja", 17336, "2f1b4d75d067bae3fe44e676721c7f077d243bc007156cb9c2f8b5836613d082", false},
			{"config.json", 6395, "6397cb6eca41b911d1dcab74e17941351057bd759284052a2331918ff6f9246c", false},
			{"generation_config.json", 208, "d4226bbe3117d2d253ba4609720ba82c6c4ce4627a9a6ae05387c78983ac03de", false},
			{"model.safetensors", 3550670554, "038e39a37a7667373d2c3991375446b10c96ae1d717a68674870343db376b76e", true},
			{"model.safetensors.index.json", 218323, "edb157dbf495e23f37377af4a628a9ad13c4ee7937f93ccb36ec9e9a19940f16", false},
			{"processor_config.json", 1316, "de3e580aebdc98272d4c4547daffe6525fcbae18a83a0e0bcf0d7444d4ee6f37", false},
			{"tokenizer.json", 32169626, "cc8d3a0ce36466ccc1278bf987df5f71db1719b9ca6b4118264f45cb627bfe0f", true},
			{"tokenizer_config.json", 2740, "080d9e1aff284e2f6043889cd05367966f7c7b80e025fbc0b06745e218158656", false},
		},
	},
	{
		ID: LLMLFM2_8B, Kind: LLM, Runtime: MLX, Title: "LFM2 8B-A1B · 3-bit", Experimental: true,
		Family: "lfm2", Quant: "3bit",
		Repo: "mlx-community/LFM2-8B-A1B-3bit-MLX", Revision: "0b05ca1d3fe12b7c9b9b57df674752eb63a46e5f",
		Files: []File{
			{"chat_template.jinja", 1296, "89e790f027916b5a2bca145a6a8454e06ffc7a5043bf3b6d97829aff86bb543f", false},
			{"config.json", 6419, "5353db612ceba7c00bf53471a6e59255fde8c6e1d61b3667a29dc3bfc5e5a572", false},
			{"generation_config.json", 137, "406488db3037c026bac5b4dde1436fb4649bbc50e5e4a2fa01115066ae96e850", false},
			{"model.safetensors", 4171677477, "aef1f8f4ada85e01e7ff07cb413d34c9e565d7becd537846b911cad15305a220", true},
			{"model.safetensors.index.json", 43344, "fe80257bee73495f4bc38749b78ea4171f544b686258d9764005d7784bebeb91", false},
			{"special_tokens_map.json", 434, "742aefe2b7dec496e8caffdba03a75d0c1a9925d53bd3f3e0d388c96b591b6f4", false},
			{"tokenizer.json", 4733020, "d7a0ab0fc22e41ec8c6d7450a9ff9ce40e196ec5e5a2fa6a2105e064e0514ed7", false},
			{"tokenizer_config.json", 91876, "8cba5b0c7acab23a0d4cc9ac587346c9220a1b6d288fc5346fe118202fd6f43e", false},
		},
	},
	{
		ID: LLMLFM2, Kind: LLM, Runtime: MLX, Title: "LFM2 1.2B · 4-bit", Experimental: true,
		Family: "lfm2", Quant: "4bit",
		Repo: "mlx-community/LFM2-1.2B-4bit", Revision: "3843e4ad0fcb8b7ed8a050908ac8f0bb5320d1bf",
		Files: []File{
			{"chat_template.jinja", 209, "a805e50fed68938a076b07e2e602639611b50b1ced0e50f11eb92f1ba25be4dc", false},
			{"config.json", 1254, "8dedff7da73f9f659acf40521f3bbe323e23c454e3a0cdc35576ddf05d301bfb", false},
			{"generation_config.json", 137, "0f678f07a00ae4bdeb2b2bd5f37988a4b33a87686d9103fefe08de7a46522373", false},
			{"model.safetensors", 658540250, "575bdb17dcfcd09a84ba702983984683584ad612ba006d05679ef20460442269", true},
			{"model.safetensors.index.json", 23414, "3074009e9be56358bf8edc25354572cbca2b5a625e02f8a2c2789a656f51f5a1", false},
			{"special_tokens_map.json", 434, "742aefe2b7dec496e8caffdba03a75d0c1a9925d53bd3f3e0d388c96b591b6f4", false},
			{"tokenizer.json", 4732426, "98cff83b4f6d7e9d8929bebc62b07e92cf1b3f99c80d16bafe8b84a75448f40b", false},
			{"tokenizer_config.json", 91509, "36f511115e9d8952cbc9d15d9a20dfa7ce7d1444940e5c1dc42a762020c99bf5", false},
		},
	},
}

// The Whisper checkpoints' tokenizers (sglang_omni_mlx/native/ci/
// model_pins.json's "tokenizer"; the family pins table's aux): MLX's
// conversions carry only config.json and the weights, and whisper_server
// reads these four files beside them (Manager.RunDir). large-v3-turbo
// and large-v3 share one.
var (
	whisperLargeV3Tokenizer = []Aux{{Repo: "openai/whisper-large-v3", Revision: "06f233fe06e710322aca913c1bc4249a0d71fce1", Files: []File{
		{"generation_config.json", 3903, "fbdfa70135de9b1d31553393f14e80aaeb1936ea36576b2ba864055943c09d23", false},
		{"merges.txt", 493869, "2df2990a395e35e8dfbc7511e08c12d56018d8d04691e0133e5d63b21e154dc6", false},
		{"tokenizer_config.json", 282843, "844b642c73a91359722f47b35705f7174686df33d252695d8572cf9ac03a6389", false},
		{"vocab.json", 1036558, "e2aa043ef015641d363d8288e7c241c85e36a5c761fb303598e0710233344387", false},
	}}}
	whisperSmallTokenizer = []Aux{{Repo: "openai/whisper-small", Revision: "973afd24965f72e36ca33b3055d56a652f456b4d", Files: []File{
		{"generation_config.json", 3868, "71565b8ef50d0bf7a1193ed4bbed195b94e70c18894d81bba2f1233dcec3ab53", false},
		{"merges.txt", 493869, "2df2990a395e35e8dfbc7511e08c12d56018d8d04691e0133e5d63b21e154dc6", false},
		{"tokenizer_config.json", 282683, "2a4c4281cf9f51ac6ccc406fdc711a087afe6530f671fa7b80953edc498275ce", false},
		{"vocab.json", 835550, "8f680bba319e01a653d2e8a5dbc17a9157179e0576e6ce74ce0c06356c6e24f9", false},
	}}}
)
