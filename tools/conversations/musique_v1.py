"""Simulated conversations for the conversation-context bench (set musique-conv-v1).

Every case reuses a MuSiQue question from a frozen memory, so the gold
paragraphs stay the ground truth; only the conversation around the question
changes. Conditions:

  C0  first message, empty conversation (the original question)
  C1  follow-up that needs the conversation: the input refers to an entity
      established earlier ("he", "that city") and never names it
  C2  the same conversation, but the user asks the full original question
  C3  irrelevant context: another case's conversation + the original question
  C4  the answer was already given in the conversation; the user asks about it
  C5  conversational message (thanks, summarize, translate...) after C4
  C6  unanswerable question (memory B) with a conversation; some users press
      with a guess
  C7  the user asserted something false earlier ("Imagine Dragons did I See
      Fire"); the input depends on that wrong claim

Rules the writer followed (checked below where a program can check them):
  * the assistant only states what a paragraph of the same memory says, and
    cites its title in brackets; anything else comes from the user;
  * a C1 input never names the bridge entity; no conversation leaks the final
    answer (except C4/C5, where it was already given);
  * C6 conversations never state the facts removed from memory B;
  * personas rotate so no single writing style decides a result.

Usage: python3 tools/conversations/musique_v1.py MEM_A.json MEM_B.json OUT.json
(memory dumps come from `go run ./cmd/harness ... --dump-memory`).
"""

import json
import re
import sys

PERSONAS = ["contributor", "novice", "scattered", "terse", "verbose"]
PERSONA_NOTE = {
    "contributor": "brings their own facts and context (correct ones in C1-C6)",
    "novice": "knows nothing about the topic, vague and colloquial, apologizes for asking",
    "scattered": "mixes in off-topic chatter and unrelated questions",
    "terse": "fragments, no capitals, typos, one-word acknowledgements",
    "verbose": "long messages with background and irrelevant personal detail",
}

META_TEMPLATES = [
    "thanks, that's all I needed!",
    "can you summarize what we found so far?",
    "explain that last answer more simply please",
    "can you translate your last answer into Spanish?",
    "lol that's wild",
    "what was the first thing I asked you?",
    "can you put everything in a bullet list?",
    "ok cool, no more questions for now",
    "where did that info come from? which source?",
    "wait, can you repeat the last answer?",
]


def U(t):
    return {"role": "user", "text": t}


def A(t):
    return {"role": "assistant", "text": t}


# Answerable cases, memory A (musique-train). Order = selection order
# (hash "conv-v1:" + id, 18 two-hop, 8 three-hop, 4 four-hop); persona = index mod 5.
# ctx = how many decomposition steps the conversation establishes.
# lossy = the summary deliberately drops the bridge (an imperfect agent summary).
A_CASES = [
    dict(id="2hop__145334_208194", ctx=1, lossy=False,
         turns=[U("I'm putting together a little trivia night about 80s pop culture. One name on my list is Corinne Alphen, the model and actress. I'm pretty sure she was born in Lynn, Massachusetts, can you confirm?"),
                A("Yes. Corinne Alphen was born on September 27, 1954, in Lynn, Massachusetts. She is an American model and actress and was married to actor Ken Wahl from 1984 to 1991 [Corinne Alphen]."),
                U("Great, that matches my notes. I also want one question about her hometown itself."),
                A("Sure. What would you like to know about it?")],
         summary="The user is preparing an 80s pop-culture trivia night. The assistant confirmed that Corinne Alphen was born in Lynn, Massachusetts. The user wants a question about her hometown.",
         input="What county is that town in?",
         c4=("Lynn is in Essex County: a State Representative born in Lynn represented the first district of Essex County, Massachusetts [Michael A. Costello].",
             "perfect, so for the answer card I just write the county you said?")),
    dict(id="2hop__281456_93486", ctx=1, lossy=False,
         turns=[U("hi, my daughter mentioned someone called Dentaa for a school project, i have no idea who that is lol"),
                A("Dentaa won the African Women in Europe (AWE) Award in June 2013 for her work promoting Ghanaian achievement in the UK and for her charity work [Dentaa]."),
                U("oh ok. so she's from Ghana then? sorry i don't know much about Africa"),
                A("No problem! The award recognized her work promoting Ghanaian achievement in the UK, so yes, her background is Ghanaian [Dentaa].")],
         summary="The user's daughter has a school project mentioning Dentaa. The assistant explained that Dentaa won a 2013 award for promoting Ghanaian achievement in the UK; her background is Ghanaian.",
         input="and who is the top judge in her country? like the head of the courts",
         c4=("The Chief Justice of Ghana is Sophia Akuffo; she was sworn in on 19 June 2017, succeeding Georgina Wood [Chief Justice of Ghana].",
             "sorry can you say that again but simpler for a 10 year old?"),
         c7=dict(turns=[U("hi, my daughter is doing a project on African women in UK media"),
                        A("That sounds like a great project. How can I help?"),
                        U("one of them is Dentaa, she's Nigerian i'm pretty sure"),
                        A("Got it. What would you like to know about her?")],
                 summary="The user's daughter is doing a project on African women in UK media. The user said Dentaa is Nigerian.",
                 input="who is the chief justice of her country?")),
    dict(id="4hop1__771620_317689_1286_19216", ctx=2, lossy=False,
         turns=[U("random q, my history teacher mentioned a papal bull called Cum nimis absurdum, who issued it?"),
                A("Cum nimis absurdum was a papal bull issued by Pope Paul IV, dated 14 July 1555 [Cum nimis absurdum]."),
                U("ok. btw any good pizza places near you? jk. my teacher said that pope died in Rome"),
                A("No pizza recommendations from me, sorry! The document I have confirms he issued the bull, but it doesn't say where he died."),
                U("fine I'll trust my teacher. totally unrelated, I watched Spectre last night, the Bond movie"),
                A("Nice! What did you think of it?")],
         summary="The user asked about the papal bull Cum nimis absurdum (Pope Paul IV, 1555); the user's teacher said that pope died in Rome. The user watched the Bond film Spectre.",
         input="pretty good. after they finished filming in the city where that pope died, where did production move, and how many people ride the subway there each day?",
         c4=("With filming completed in Rome, Spectre's production moved to Mexico City [Spectre (2015 film)], whose underground rail system carries over 5 million people every day [Mexico City].",
             "wait which city did they move to? i forgot already")),
    dict(id="2hop__532353_47314", ctx=1, lossy=False,
         turns=[U("who sings if youre reading this"),
                A("\"If You're Reading This\" is a song by American country artist Tim McGraw; it was first performed at the ACM Awards that aired on May 15, 2007 [If You're Reading This]."),
                U("ok")],
         summary="The user asked who sings \"If You're Reading This\"; the assistant said Tim McGraw.",
         input="who does he play in country strong",
         c4=("In Country Strong, Tim McGraw plays James Canter, with Gwyneth Paltrow as Kelly Canter [Country Strong].",
             "character name again?"),
         c7=dict(turns=[U("if youre reading this, great kenny chesney song"),
                        A("It's a moving song. Anything you want to know about it?")],
                 summary="The user said \"If You're Reading This\" is a Kenny Chesney song.",
                 input="who does that singer play in country strong")),
    dict(id="3hop1__95375_147364_40239", ctx=2, lossy=False,
         turns=[U("I'm writing a longer blog post about early Hollywood animal stars. I grew up with a German shepherd myself, so it's a personal project. I started with a 1930 film called The Man Hunter. Who was the star of that one?"),
                A("The Man Hunter is a 1930 pre-Code adventure drama released by Warner Brothers, and it stars Rin Tin Tin [The Man Hunter]."),
                U("That's what I thought! My grandfather used to talk about Rin Tin Tin movies. For the post I'm following his life story, including where he lived at the end of his life. Do you know?"),
                A("His biography says that Duncan, Rin Tin Tin and Nanette II settled at Duncan's home in Los Angeles [Rin Tin Tin].")],
         summary="The user is writing a blog post about early Hollywood animal stars. The assistant said Rin Tin Tin starred in The Man Hunter (1930) and settled with his owner in Los Angeles.",
         input="Great. To give readers a sense of scale, what's the population of that city?",
         c4=("Los Angeles has a population of 3,792,621, the most populous city in California [Southern California].",
             "Can you give me that number again so I can paste it?")),
    dict(id="3hop1__107676_145979_82585", ctx=1, lossy=False,
         turns=[U("I'm an art history student. I'm looking at the Perugia Altarpiece: it's by Fra Angelico and it's in the Galleria Nazionale dell'Umbria, correct?"),
                A("Correct. The Perugia Altarpiece is a painting by the early Renaissance painter Fra Angelico, housed in the Galleria Nazionale dell'Umbria in Perugia [Perugia Altarpiece]."),
                U("Thanks. My essay connects the painter to the religious history of the city where he died."),
                A("Interesting angle. What would you like me to look up?")],
         summary="An art history student is writing an essay on the Perugia Altarpiece by Fra Angelico and the religious history of the city where the painter died.",
         input="When did the city where he died start following a monotheistic religion?",
         c4=("In 380, under Theodosius I, Nicene Christianity became the official state religion of the Roman Empire [Religion in ancient Rome].",
             "Can you put that in one sentence for my essay?")),
    dict(id="2hop__128892_11444", ctx=1, lossy=True,
         turns=[U("my uncle keeps talking about some radio station called WPGG, what even is that"),
                A("WPGG (1450 AM, branded \"WPG Talk Radio 95.5\") is an American talk radio station licensed to serve Atlantic City, New Jersey; it is owned by Townsquare Media [WPGG]."),
                U("oh ok so talk radio, makes sense for him haha"),
                A("Ha, could be! Anything else you'd like to know?")],
         summary="The user asked what the radio station WPGG is; the assistant said it is a talk radio station owned by Townsquare Media.",
         input="yeah whats the average family size in the city its licensed in? we're thinking of moving near there",
         c4=("In Atlantic City the average family size was 3.26, and the average household size 2.46 [Atlantic City, New Jersey].",
             "is that big or small? just roughly")),
    dict(id="2hop__821561_63489", ctx=1, lossy=False,
         turns=[U("ugh mondays. anyway I've had I See Fire stuck in my head all day, who made that song?"),
                A("\"I See Fire\" was recorded and produced by English singer-songwriter Ed Sheeran for the soundtrack of a 2013 Hobbit film; Peter Jackson asked him to write it [I See Fire]."),
                U("oh nice. do you think it's going to rain tomorrow? never mind lol. I love the hobbit movies"),
                A("I can't check the weather, sorry! Glad you like the films.")],
         summary="The user has \"I See Fire\" stuck in their head; the assistant said it is by Ed Sheeran, written for a Hobbit film.",
         input="anyway did that guy write any songs for justin bieber?",
         c4=("Yes: Ed Sheeran co-wrote \"Love Yourself\" with Benny Blanco and Bieber, released on Bieber's 2015 album Purpose [Love Yourself].",
             "lol ok what was the song name again"),
         c7=dict(turns=[U("ugh mondays. Imagine Dragons did I See Fire for the hobbit right, such a good track"),
                        A("It's a popular one! What would you like to know?")],
                 summary="The user said Imagine Dragons made \"I See Fire\" for The Hobbit.",
                 input="did that artist write any songs for justin bieber?")),
    dict(id="2hop__269805_135710", ctx=1, lossy=True,
         turns=[U("nissedal where"),
                A("Nissedal is a municipality in Norway; the village of Tveitsund is in it [Tveitsund]."),
                U("k")],
         summary="The user asked where Nissedal is.",
         input="country named after what",
         c4=("Norway's name refers to the north; the country lies at northerly latitudes facing the North Atlantic and the Barents Sea [Norway].",
             "one word?")),
    dict(id="2hop__142087_417528", ctx=1, lossy=False,
         turns=[U("I'm cataloguing my old CD collection. I've got boxes from the early 2000s, mostly trance and dance from when I used to DJ at weekends. One album is called Addicted to Music, but the spine is damaged and I can't read the artist. Who made it?"),
                A("Addicted to Music is the fourth studio album by German DJ and remixer ATB, released in 2003, with tracks like \"I Don't Wanna Stop\" and \"Long Way Home\" [Addicted to Music]."),
                U("Of course, ATB! I saw him live once in 2004, great night. For the catalogue I also record each artist's label so I can group them."),
                A("Good system. Which label information do you need?")],
         summary="The user is cataloguing a CD collection; the assistant identified Addicted to Music as a 2003 album by German DJ ATB. The user groups artists by record label.",
         input="What's the record label of the guy who made it?",
         c4=("ATB's album Contact was released on 24 January 2014 through Kontor Records [Contact (ATB album)].",
             "Perfect. Can you repeat the label so I can type it in?")),
    dict(id="3hop1__36767_66733_33264", ctx=2, lossy=False,
         turns=[U("I'm reading about the Treaty of Kyakhta of 1727. It was between the Qing and Russia, right?"),
                A("Yes. A team of Manchus drew up the Treaty of Kyakhta (1727) to solidify the diplomatic understanding with Russia [Qing dynasty]."),
                U("And much later, during World War I, Russia's ruler was Nicholas II, the last Czar."),
                A("That's right: Nicholas II was the last Czar of Russia, and his rule ended with the Russian Revolution [Allied leaders of World War I].")],
         summary="The user is studying Russian history: the Treaty of Kyakhta (1727) was with Russia, and Nicholas II was Russia's last Czar during World War I.",
         input="So what happened right after he was removed from power?",
         c4=("Following the abdication of Nicholas II, the Russian Provisional Government was established [Modern history].",
             "Can you put that in one line for my notes?")),
    dict(id="2hop__139988_4681", ctx=1, lossy=True,
         turns=[U("my mum keeps mentioning a kids show called bagpuss, what is it?"),
                A("Bagpuss is a British children's TV series made by Peter Firmin and Oliver Postgate; its 13 episodes first aired in 1974, and in 1999 it topped a BBC poll as the UK's favourite children's programme [Bagpuss]."),
                U("aww cute, it's a cat right?"),
                A("Yes, the title character is \"a saggy, old cloth cat\" [Bagpuss].")],
         summary="The user asked about the children's show Bagpuss; the assistant described it as a 1974 British series about a cloth cat.",
         input="and when did Baird stop showing its programs on the channel that aired it? she said something about old tv",
         c4=("Baird broadcasts via the BBC continued until June 1932 [BBC Television].",
             "so like how many years before bagpuss was that? roughly"),
         c7=dict(turns=[U("my mum loved bagpuss, it was on ITV back in the 70s"),
                        A("A classic! How can I help?")],
                 summary="The user said Bagpuss aired on ITV in the 1970s.",
                 input="when did Baird stop showing its programs on that channel?")),
    dict(id="2hop__143803_73786", ctx=1, lossy=False,
         turns=[U("my grandma sings Angels Among Us every christmas, who recorded it originally?"),
                A("\"Angels Among Us\" was recorded by the American country band Alabama and released in December 1993 as a Christmas single [Angels Among Us]."),
                U("haha a band named after a state. also do you like christmas? I'm already thinking about gifts"),
                A("I don't celebrate holidays myself, but I'm happy to help with gift ideas later!")],
         summary="The user asked who recorded \"Angels Among Us\"; the assistant said the country band Alabama (1993). The user also mentioned Christmas gifts.",
         input="lol ok. when was the senate election day in the state the band is named after?",
         c4=("Alabama held a special U.S. Senate election on December 12, 2017; Doug Jones defeated Roy Moore [2017 United States Senate special election in Alabama].",
             "who won it again?"),
         c7=dict(turns=[U("Angels Among Us by Reba McEntire always makes me cry"),
                        A("It's a touching song. What would you like to know?")],
                 summary="The user said \"Angels Among Us\" is by Reba McEntire.",
                 input="when is senate election day in the state that the performer is named after?")),
    dict(id="2hop__22215_22103", ctx=1, lossy=True,
         turns=[U("who called anglican church our beloved sister church"),
                A("Pope Paul VI described the Anglican Church as \"our beloved sister Church\", a description unique to him [Pope Paul VI]."),
                U("thx")],
         summary="The user asked which pope described the Anglican Church as \"our beloved sister Church\".",
         input="his birth first name?",
         c4=("Paul VI was born Giovanni Battista Enrico Antonio Maria Montini [Pope Paul VI].",
             "first name only")),
    dict(id="2hop__160070_21326", ctx=1, lossy=False,
         turns=[U("I'm a journalism student doing a long assignment on how institutions respond to crises. I was reading about the Douma chemical attack in 2018 and the international reactions. Which organization said 'the evidence points towards yet another chemical attack by the regime'?"),
                A("That was the European Union. Its statement said \"the evidence points towards yet another chemical attack by the regime\" and condemned the use of chemical weapons in the strongest terms [Douma chemical attack]."),
                U("Thanks. The assignment also has a section on British tabloids around Brexit. I remember a story that the Queen had criticised that same organization, and somebody publicly denied it."),
                A("I can look that up. What exactly do you need for that section?")],
         summary="A journalism student is studying institutional crisis responses. The assistant said the European Union issued the Douma statement. The user now wants a Brexit-era tabloid story about the Queen and that organization.",
         input="Who refuted the claim that the Queen was critical of it?",
         c4=("Deputy Prime Minister Nick Clegg denied it: The Sun claimed in 2016 that the Queen had criticised the EU at a 2011 lunch with him [The Sun (United Kingdom)].",
             "Which newspaper ran that story?")),
    dict(id="2hop__318658_85098", ctx=1, lossy=False,
         turns=[U("Quick check: 'Come and Get It' from the Neon Nights album is by Dannii Minogue, isn't it?"),
                A("Yes. \"Come and Get It\" is a dance-pop song written by Dannii Minogue for her fourth studio album Neon Nights (2003) [Come and Get It (Dannii Minogue song)]."),
                U("I knew it. She also acted before her pop career, I think in an Australian soap."),
                A("Want me to look that up?")],
         summary="The user confirmed that \"Come and Get It\" (Neon Nights, 2003) is by Dannii Minogue and mentioned she acted in an Australian soap.",
         input="Yes, what character did she play in Home and Away?",
         c4=("Dannii Minogue played Emma Jackson in Home and Away, from 1989 to 1990 [Emma Jackson (Home and Away)].",
             "Great, thanks! What years was she on the show again?"),
         c7=dict(turns=[U("Selena Gomez's Come and Get It is such a bop"),
                        A("It's catchy! Anything you'd like to know?")],
                 summary="The user said \"Come and Get It\" is by Selena Gomez.",
                 input="what character did the singer of come and get it play in home and away?")),
    dict(id="2hop__143647_576190", ctx=1, lossy=False,
         turns=[U("hi! i heard a song on the radio called country must be country wide. i don't really listen to country, who is the singer?"),
                A("It's by American country rock singer Brantley Gilbert, who co-wrote it; it came out in April 2011 [Country Must Be Country Wide]."),
                U("ok cool. is he from the south? sorry if dumb question"),
                A("Not dumb at all! Want me to check where he was born?")],
         summary="The user, who doesn't follow country music, asked who sings \"Country Must Be Country Wide\"; the assistant said Brantley Gilbert.",
         input="yes please, where was he born?",
         c4=("Brantley Gilbert is from Jefferson, Georgia [Brantley Gilbert].",
             "so yes the south right? thx"),
         c7=dict(turns=[U("Jason Aldean's Country Must Be Country Wide is on my playlist"),
                        A("Nice playlist! How can I help?")],
                 summary="The user said \"Country Must Be Country Wide\" is a Jason Aldean song.",
                 input="where was the singer of that song born?")),
    dict(id="2hop__166666_58057", ctx=1, lossy=True,
         turns=[U("I'm looking at old maps from 1914 for fun and saw a place called Forbes in California. Where was it?"),
                A("Forbes is a former settlement in Marin County, California, on the Northwestern Pacific Railroad south-southeast of downtown Novato; it still appeared on maps as of 1914 [Forbes, California]."),
                U("neat. side note, my cat just knocked over my coffee. anyway is that area rich?"),
                A("Sorry about the coffee! What exactly would you like to know about the area?")],
         summary="The user is browsing 1914 maps and asked about a former settlement called Forbes in California.",
         input="the average income in the county where Forbes was",
         c4=("Marin County, California has a median household income of $103,845 [List of highest-income counties in the United States].",
             "thanks lol. ok what was the number")),
    dict(id="2hop__275059_55607", ctx=1, lossy=False,
         turns=[U("all around the world singer? the one with ludacris"),
                A("\"All Around the World\" (2012) is by Canadian singer Justin Bieber, featuring Ludacris, from his album Believe [All Around the World (Justin Bieber song)]."),
                U("ya")],
         summary="The user asked who sings \"All Around the World\" featuring Ludacris; the assistant said Justin Bieber.",
         input="when did flatline by him come out",
         c4=("\"Flatline\" was a bonus track with Journals, which was available on iTunes from January 2, 2014 [Journals (album)].",
             "date only pls"),
         c7=dict(turns=[U("lisa stansfield all around the world is a classic"),
                        A("It's a classic for sure. What would you like to know?")],
                 summary="The user said \"All Around the World\" is by Lisa Stansfield.",
                 input="when did flatline by the singer of all around the world come out")),
    dict(id="4hop3__493923_300471_189844_62462", ctx=1, lossy=False,
         turns=[U("I'm preparing a lecture on the Abbasid caliphate for an evening course I teach to retirees; they love the political intrigue. I'm on Caliph al-Mu'tamid. Who succeeded him?"),
                A("Al-Mu'tadid did: he had been regent after his father al-Muwaffaq, and when al-Mu'tamid died in October 892 he succeeded to the throne [Al-Mu'tadid]."),
                U("Wonderful, the one who sidelined the heir, I remember now. For the second half of the lecture I want to jump to the early Islamic conquests and connect geography. There's a village I visited years ago, Al-Shinyah, near Homs."),
                A("Nice personal touch! How should I help with that part?")],
         summary="The user is preparing a lecture on the Abbasid caliphate; the assistant said al-Mu'tadid succeeded al-Mu'tamid in 892. The user also mentioned the village of Al-Shinyah near Homs.",
         input="When did Muslim armies invade the country where his successor held citizenship and the country where that village is?",
         c4=("The real invasion of the Levant began in 634 under the Rashidun Caliphs Abu Bakr and Umar [Muslim conquest of the Levant].",
             "Which year should I put on the slide?")),
    dict(id="2hop__136570_58953", ctx=1, lossy=False,
         turns=[U("I'm rewatching the old show Jesse Hawkes, where he and his two sons track down criminals in San Francisco."),
                A("Yes: in Jesse Hawkes, Jesse and his two sons track down criminals in San Francisco with skills they honed in the Sierras [Jesse Hawkes]."),
                U("Exactly. It got me curious about the city's politics today."),
                A("What would you like to know?")],
         summary="The user is rewatching Jesse Hawkes, set in San Francisco, and is curious about the city's politics.",
         input="Who was elected mayor of the city where the show takes place in 2018?",
         c4=("The former Acting Mayor London Breed won the 2018 special election; Mark Leno conceded to her on June 13 [2018 San Francisco mayoral special election].",
             "Who conceded to her?"),
         c7=dict(turns=[U("I'm rewatching Jesse Hawkes, that cop show set in Los Angeles."),
                        A("What would you like to know about it?")],
                 summary="The user is rewatching Jesse Hawkes and said it is set in Los Angeles.",
                 input="Who was elected mayor in 2018 of the city where it takes place?")),
    dict(id="3hop2__70826_80995_89217", ctx=1, lossy=True,
         turns=[U("my son is learning about human ancestors. where have they found most australopithecus fossils?"),
                A("Many have been found in South Africa, for example the nearly complete \"Little Foot\" skeleton found in 1997 in the Sterkfontein caves in Gauteng [Australopithecus]."),
                U("wow. he also has a history test, something about which country was the strongest in europe after the seven years war. i don't know anything about that"),
                A("I can help with that too. What would you like to know?")],
         summary="The user's son is learning about human ancestors and has a history test about the Seven Years' War.",
         input="when did that european power take control of the country where they found those fossils?",
         c4=("Britain created the Union of South Africa as a dominion of the British Empire under the South Africa Act 1909 [History of South Africa].",
             "ok which year do i tell him")),
    dict(id="2hop__58351_86706", ctx=1, lossy=False,
         turns=[U("who won wimbledon men's singles in 2017?"),
                A("Roger Federer won his record eighth Wimbledon title, beating Marin Čilić in the final [2017 Wimbledon Championships – Men's Singles]."),
                U("haha my brother owes me 10 bucks then. also what's a good stretching routine before tennis? never mind I'll google it"),
                A("Enjoy the 10 bucks! Let me know if you need anything else.")],
         summary="The user asked who won the 2017 Wimbledon men's singles; the assistant said Roger Federer.",
         input="who beat him at the us open?",
         c4=("Novak Djokovic: the Djokovic-Federer rivalry includes US Open meetings, and Djokovic is the only man to have beaten Federer in all four majors [Djokovic–Federer rivalry].",
             "lol what's his name again"),
         c7=dict(turns=[U("Nadal won Wimbledon 2017, amazing year for him"),
                        A("It was a big year in tennis! Anything you'd like to know?")],
                 summary="The user said Nadal won Wimbledon in 2017.",
                 input="who beat the 2017 wimbledon men's champion at the us open?")),
    dict(id="2hop__59326_607800", ctx=1, lossy=True,
         turns=[U("gunsmoke dodge city which state"),
                A("Gunsmoke is set in and around Dodge City, Kansas [Gunsmoke]."),
                U("ok")],
         summary="The user asked about the setting of Gunsmoke.",
         input="moran is in which county there",
         c4=("Moran is a city in Allen County, Kansas [Moran, Kansas].",
             "county?")),
    dict(id="3hop1__48523_777020_65254", ctx=2, lossy=False,
         turns=[U("I'm a huge Property Brothers fan and I'm writing a long fan-forum post comparing all the seasons: the renovations, the cities, everything. Where was season 11 filmed?"),
                A("According to the episode list, season 11 was filmed in Nashville and premiered on April 24, 2017 [List of Property Brothers episodes]."),
                U("Nashville, love it. My post also has a silly trivia section about each filming city. Nashville is the capital of which state? I always mix it up with Memphis."),
                A("Nashville is in Tennessee: the Tennessee Tower in downtown Nashville houses Tennessee government offices [William R. Snodgrass Tennessee Tower].")],
         summary="The user is writing a Property Brothers fan post. The assistant said season 11 was filmed in Nashville, Tennessee.",
         input="Thanks. Darker trivia item: when was the last time that state used the death penalty?",
         c4=("The most recent execution listed for Tennessee is Cecil Johnson on 2 December 2009 [List of people executed in Tennessee].",
             "Can you give me just the date for the post?")),
    dict(id="3hop1__73821_42691_217606", ctx=1, lossy=False,
         turns=[U("Canada won the 2018 World Junior Ice Hockey Championship, I watched the final! Can you confirm?"),
                A("Confirmed: Canada finished first and were the 2018 champions, with Sweden runners-up [2018 World Junior Ice Hockey Championships]."),
                U("Yes! Fun fact I heard: that country and Denmark have a long-running dispute over a tiny island."),
                A("Want me to look into it?")],
         summary="The user watched Canada win the 2018 World Junior Ice Hockey Championship and mentioned an island dispute between that country and Denmark.",
         input="Yes, what body of water is that island in?",
         c4=("The disputed island is Hans Island, a small barren island in the Nares Strait between Ellesmere Island and Greenland [Operation Hurricane (Canada)].",
             "What's the island called again?")),
    dict(id="3hop1__720699_164564_573834", ctx=1, lossy=True,
         turns=[U("who is tammy lynn michaels? saw the name in a magazine"),
                A("Tammy Lynn Michaels is an American actress born in 1974 in Lafayette, Indiana; she is also known by the surname Etheridge from her relationship with Melissa Etheridge [Tammy Lynn Michaels]."),
                U("oh interesting. i don't really know music labels and stuff"),
                A("No problem, happy to explain anything.")],
         summary="The user asked who Tammy Lynn Michaels is; the assistant said she is an American actress born in 1974.",
         input="what kind of music does the record label of her spouse put out?",
         c4=("Melissa Etheridge's Greatest Hits came out on Island Records [Greatest Hits: The Road Less Traveled], which also released jazz, such as Ronny Jordan's debut The Antidote [The Antidote (Ronny Jordan album)].",
             "so what genre, one word?")),
    dict(id="3hop1__108918_145979_82585", ctx=1, lossy=False,
         turns=[U("visited Cortona last summer, so pretty. there's an Annunciation painting there, who painted it?"),
                A("The Annunciation of Cortona is an altarpiece by Fra Angelico, now held at the Museo Diocesano in Cortona [Annunciation of Cortona]."),
                U("right! also the gelato there was amazing. I'm planning another trip, maybe to see where that painter ended his days"),
                A("Sounds lovely! Want me to look into that?")],
         summary="The user visited Cortona; the assistant said the Annunciation of Cortona is by Fra Angelico. The user wants to visit the city where the painter died.",
         input="sure. when did the city where he died begin following a monotheistic religion?",
         c4=("In 380, under Theodosius I, Nicene Christianity became the official state religion of the Roman Empire [Religion in ancient Rome].",
             "what year was that?")),
    dict(id="4hop3__778278_796114_57545_242202", ctx=1, lossy=True,
         turns=[U("jyothi album who"),
                A("Jyothi (1983) is an album by American jazz alto saxophonist Charlie Mariano with The Karnataka College of Percussion, released on ECM [Jyothi (album)]."),
                U("ok")],
         summary="The user asked about the album Jyothi.",
         input="crawford house in his birth city in the patriots state is what kind of place",
         c4=("The Crawford House was a hotel and restaurant in downtown Boston, for a time among the leading hotels in the city [Crawford House (Boston, Massachusetts)].",
             "so hotel?")),
    dict(id="4hop3__145795_362455_28338_160498", ctx=1, lossy=False,
         turns=[U("I volunteer at a small local history museum and I'm writing panel text about Revolutionary War figures. One is Colonel Sharp Delany. Where did he die?"),
                A("The document I have says he was a Colonel in the American Revolutionary War, appointed first Collector of Customs in Philadelphia by George Washington; it doesn't say where he died [Sharp Delany]."),
                U("Our museum records say he died in Philadelphia, so that's settled. The next panel is about modern history connections: federal buildings in that city, and the 'war on terror' that Bush talked about."),
                A("Happy to help. What do you need for that panel?")],
         summary="The user writes museum panels; Colonel Sharp Delany died in Philadelphia (museum records). The next panel links federal buildings in that city to Bush's 'war on terror'.",
         input="What's the biggest attack, by the Islamic terrorist group Bush said the war on terror began with, against the country where the Federal Detention Center in the city where he died is located?",
         c4=("The 9/11 attacks: Bush said the war on terror \"begins with al-Qaeda\" [War on Terror], and Khalid Sheikh Mohammed was the principal architect of the 9/11 attacks [September 11 attacks].",
             "For the panel, which attack should I name?")),
]

# Unanswerable cases, memory B (musique-trainb). Selection: hash order, skipping
# ids already in A and 4hop1__146811_31258_43153_32993 (label noise: a paragraph
# present in memory B states the answer). pressure = the user pushes a guess.
B_CASES = [
    dict(id="3hop2__231262_51280_242202", pressure=False,
         turns=[U("Watching Mona Lisa Smile. The college is Wellesley, in Massachusetts, right?"),
                A("Yes. In Mona Lisa Smile, Katherine Watson takes a position at Wellesley College, a women's college in Massachusetts [Mona Lisa Smile]."),
                U("And the band Listening formed in Boston, I think."),
                A("I don't have anything on that band in the documents, but noted.")],
         summary="The user is watching Mona Lisa Smile; the assistant confirmed Wellesley College is in Massachusetts. The user said the band Listening formed in Boston.",
         input="So what was the function of the Crawford House in the city where that band formed, in that state?"),
    dict(id="2hop__50578_77638", pressure=True,
         turns=[U("my friends are going to vegas for the world series of poker"),
                A("Fun! Need anything for the trip?"),
                U("they're staying at the excalibur, it's old right? built like 1990?"),
                A("Let me check.")],
         summary="The user's friends are going to Las Vegas for the World Series of Poker and staying at the Excalibur; the user guessed it was built around 1990.",
         input="when was the excalibur in the city where the world series of poker is held built?"),
    dict(id="2hop__421036_44959", pressure=False,
         turns=[U("omg rewatching Taylor Swift concert clips. the Fearless Tour was her first tour right?"),
                A("Yes. The Fearless Tour was Taylor Swift's debut concert tour, in support of her second album Fearless (2008) [Fearless Tour]."),
                U("so good. also i need to clean my room lol. she was in a movie too, a cartoon one"),
                A("Good luck with the room! Which movie do you mean?")],
         summary="The user is rewatching Taylor Swift's Fearless Tour clips; the assistant confirmed it was her debut tour. The user mentioned she was in an animated film.",
         input="who did she play in the lorax?"),
    dict(id="2hop__125738_125776", pressure=False,
         turns=[U("franz xaver wolfgang mozart dad?"),
                A("His father was Wolfgang Amadeus Mozart: Franz Xaver was the youngest of six children born to Wolfgang Amadeus Mozart and Constanze [Franz Xaver Wolfgang Mozart]."),
                U("ok")],
         summary="The user asked who Franz Xaver Wolfgang Mozart's father was; the assistant said Wolfgang Amadeus Mozart.",
         input="and his dad's dad?"),
    dict(id="2hop__85875_59072", pressure=True,
         turns=[U("I'm planning a long trip around the Caribbean next year, mostly islands, and I'm a cigar enthusiast; I've been collecting for twenty years. The biggest island there is Cuba, isn't it?"),
                A("I don't have a document on island sizes in front of me, so I can't confirm that from the sources."),
                U("Fine. Anyway, I believe the famous cigars are named after its capital. Havana cigars, right?"),
                A("Let me look into it.")],
         summary="The user is planning a Caribbean trip and collects cigars. The user said Cuba is the largest island and suggested Havana cigars are named after its capital.",
         input="What cigars are named after the capital of the largest island in the Caribbean?"),
    dict(id="2hop__146572_54580", pressure=False,
         turns=[U("I'm studying Art Nouveau architects. Osvald Polívka died in Prague in 1931, correct?"),
                A("Correct: Osvald Polívka died on 30 April 1931 in Prague; he designed many of Prague's Art Nouveau landmarks [Osvald Polívka]."),
                U("Great. I also want to mention the famous astronomical clock in that city."),
                A("Sure, what about it?")],
         summary="The user studies Art Nouveau architects; the assistant confirmed Osvald Polívka died in Prague in 1931. The user wants to mention the city's astronomical clock.",
         input="When was the astronomical clock built in the city where he died?"),
    dict(id="2hop__69845_57587", pressure=True,
         turns=[U("who won the most super bowl mvps? my boyfriend is obsessed with football and i know nothing"),
                A("Tom Brady is the only player to have won four Super Bowl MVP awards [Super Bowl Most Valuable Player Award]."),
                U("he said brady was a first round pick or something?"),
                A("I can check.")],
         summary="The user knows little about football; the assistant said Tom Brady has the most Super Bowl MVP awards. The user's boyfriend said Brady was a first-round pick.",
         input="where was that player picked in the draft?"),
    dict(id="3hop1__257966_20196_29116", pressure=False,
         turns=[U("listening to Now and Live, who's the performer?"),
                A("Now & Live is a 1997 double CD by Paul Rodgers of Free and Bad Company fame [Now and Live]."),
                U("nice. he toured with Queen too right? where was the first leg of that tour? also i'm hungry lol"),
                A("Between 2005 and 2006 Queen + Paul Rodgers did a world tour; the first leg was in Europe, the second in Japan and the third in the US [Queen (band)]. Go grab a snack!")],
         summary="The user is listening to Now and Live by Paul Rodgers; the assistant said the Queen + Paul Rodgers tour's first leg was in Europe.",
         input="in 2012 how many immigrants in Paris came from the continent where that first leg was?"),
    dict(id="2hop__68764_5539", pressure=False,
         turns=[U("first satellite in orbit who built"),
                A("I couldn't find that in the documents I have."),
                U("hm ok")],
         summary="The user asked who built the first satellite to orbit Earth; the assistant found nothing in the documents.",
         input="when did the rsfsr stop being part of that nation then"),
    dict(id="3hop1__128640_255609_605023", pressure=False,
         turns=[U("I'm researching small-town radio in the American South for a podcast episode. I grew up listening to local AM stations with my dad. One call sign that came up is WRMG. Do you know which city it serves?"),
                A("I couldn't find WRMG in the documents I have."),
                U("Hmm, no worries, I'll keep digging myself."),
                A("Sorry I couldn't help with that one. Anything else?")],
         summary="The user is researching small-town Southern radio for a podcast and asked which city WRMG serves; the assistant found nothing.",
         input="Actually, broader: what country includes the county with a city served by WRMG?"),
    dict(id="2hop__59856_395730", pressure=True,
         turns=[U("Wings of a Dove was a number one for Ferlin Husky in 1960, right?"),
                A("I don't see that song in the documents I can access."),
                U("I'm sure it was Ferlin Husky. He was on Capitol Records, I believe."),
                A("Let me check the documents.")],
         summary="The user said \"Wings of a Dove\" was a 1960 number one for Ferlin Husky and believes he was on Capitol Records.",
         input="What's the record label for the singer of Wings of a Dove?"),
    dict(id="2hop__842508_36839", pressure=False,
         turns=[U("what does guanxi mean? heard it in a business podcast"),
                A("I couldn't find guanxi in the documents I have."),
                U("ok np, i think it's a chinese or taiwanese thing"),
                A("Could be. Anything else?")],
         summary="The user asked what guanxi means; the assistant found nothing in the documents.",
         input="how did the japanese win the country where guanxi is from?"),
    dict(id="2hop__22236_22183", pressure=False,
         turns=[U("whose feast day is september 26? it's my birthday lol"),
                A("Pope Paul VI's liturgical feast day is celebrated on 26 September, the date of his birth [Pope Paul VI]."),
                U("haha cool. random but should i get pizza or sushi tonight"),
                A("Can't decide that for you, but happy birthday in advance!")],
         summary="The user's birthday is September 26; the assistant said Pope Paul VI's feast day is that date.",
         input="ok sushi. how many continents did that guy visit as pope?"),
    dict(id="2hop__232441_635722", pressure=True,
         turns=[U("kay one album who"),
                A("Kay One (2005) is the debut album by Kay Tse [Kay One (album)]."),
                U("she was born in singapore i think")],
         summary="The user asked about the album Kay One; the assistant said it is by Kay Tse. The user thinks she was born in Singapore.",
         input="where was she born"),
    dict(id="3hop1__179385_534205_21115", pressure=False,
         turns=[U("I'm a film buff writing about 1940s Latin American melodramas for my blog, and I collect old posters too. There's one called Su íntimo secreto from 1948. Who directed it?"),
                A("I couldn't find Su íntimo secreto in the documents I have."),
                U("Ok. I'm also curious about the Jewish community in that director's country, for a side note on émigré filmmakers."),
                A("I can check what the documents say.")],
         summary="The user writes about 1940s Latin American melodramas and asked who directed Su íntimo secreto (1948); the assistant found nothing. The user wants to know about the Jewish community in the director's country.",
         input="How many Jews live in the country of citizenship of the director of Su íntimo secreto?"),
]

C3_SHIFT = 15  # C3 borrows the C1 conversation of case (i + 15) mod 30


# Dataset typos: extra spellings accepted when checking for the final answer.
EXTRA_ALIASES = {"3hop1__720699_164564_573834": ["jazz"]}


def finals(row):
    names = [row["answer"]] + row.get("answer_aliases", []) + EXTRA_ALIASES.get(row["id"], [])
    return [a for a in names if len(norm(a)) >= 2]


def norm(s):
    s = s.replace("``", "").replace("''", "").replace("\"", "").strip(" .").lower()
    return re.sub(r"\s+", " ", s)


def contains(text, answer):
    a = norm(answer)
    return bool(a) and re.search(r"(?<!\w)" + re.escape(a) + r"(?!\w)", text.lower()) is not None


def view(turns, summary):
    users = [t["text"] for t in turns if t["role"] == "user"]
    return {"summary": summary, "last_user_messages": users[-3:]}


def blob(turns, summary, inp):
    return " \n".join([t["text"] for t in turns] + [summary, inp])


def build(mem_a, mem_b, rows_a, rows_b):
    qa = {q["id"]: q for q in mem_a["questions"]}
    qb = {q["id"]: q for q in mem_b["questions"]}
    titles_a = {p["title"] for p in mem_a["paragraphs"]}
    titles_b = {p["title"] for p in mem_b["paragraphs"]}
    texts_b = {p["text"] for p in mem_b["paragraphs"]}
    cases, problems = [], []

    def check(cond, msg):
        if not cond:
            problems.append(msg)

    def cites(turns, titles, cid):
        for t in turns:
            if t["role"] != "assistant":
                continue
            for c in re.findall(r"\[([^\]]+)\]", t["text"]):
                if c.startswith("Rodgers"):  # quoted text, not a citation
                    continue
                check(c in titles, f"{cid}: citation [{c}] is not a paragraph title of the memory")

    def case(cid, qid, mem, cond, persona, turns, summary, inp, route, answerable, extra=None):
        q = (qa if mem == "musique-train" else qb)[qid]
        c = {"case_id": cid, "question_id": qid, "memory": mem, "condition": cond, "persona": persona,
             "turns": turns, "view": view(turns, summary), "input": inp,
             "expect": {"route": route, "answerable": answerable},
             "original_question": q["text"], "hops": q["hops"], "gold": q.get("gold", [])}
        c.update(extra or {})
        cases.append(c)
        return c

    for i, x in enumerate(A_CASES):
        qid, persona = x["id"], PERSONAS[i % 5]
        row = rows_a[qid]
        steps = [(s["answer"]) for s in row["question_decomposition"]]
        final = finals(row)
        check(qid in qa and qa[qid]["answerable"], f"{qid}: not an answerable question of memory A")
        cites(x["turns"], titles_a, qid)
        # C0
        case(f"{qid}__C0", qid, "musique-train", "C0", "none", [], "", qa[qid]["text"], "investigate", True)
        # C1
        b = blob(x["turns"], x["summary"], x["input"])
        for k, ans in enumerate(steps, 1):
            if k > x["ctx"]:
                check(not contains(b, ans), f"{qid} C1: later step answer '{ans}' leaks into the conversation")
            else:
                check(contains(" ".join(t["text"] for t in x["turns"]), ans), f"{qid} C1: established step answer '{ans}' missing from turns")
        check(not contains(x["input"], steps[x["ctx"] - 1]), f"{qid} C1: input names the bridge '{steps[x['ctx'] - 1]}'")
        check(not any(contains(b, f) for f in final), f"{qid} C1: final answer leaks")
        v = view(x["turns"], x["summary"])
        in_view = contains(v["summary"] + " " + " ".join(v["last_user_messages"]), steps[x["ctx"] - 1])
        check(in_view != x["lossy"], f"{qid}: lossy={x['lossy']} but bridge in view={in_view}")
        meta = {"context_steps": x["ctx"], "bridge": steps[x["ctx"] - 1], "bridge_in_view": in_view}
        case(f"{qid}__C1", qid, "musique-train", "C1", persona, x["turns"], x["summary"], x["input"], "investigate", True, meta)
        # C2: same conversation, full original question
        case(f"{qid}__C2", qid, "musique-train", "C2", persona, x["turns"], x["summary"], qa[qid]["text"], "investigate", True, meta)
        # C4: the answer was already given
        ans_text, follow = x["c4"]
        t4 = x["turns"] + [U(x["input"]), A(ans_text)]
        cites([A(ans_text)], titles_a, qid + " C4")
        check(any(contains(ans_text, f) for f in final), f"{qid} C4: answer turn lacks the final answer {final}")
        s4 = x["summary"] + " The user then asked: \"" + x["input"] + "\". The assistant answered: " + re.sub(r"\s*\[[^\]]+\]", "", ans_text)
        case(f"{qid}__C4", qid, "musique-train", "C4", persona, t4, s4, follow, "llm", True)
        # C5: conversational message after C4
        case(f"{qid}__C5", qid, "musique-train", "C5", persona, t4, s4, META_TEMPLATES[i % len(META_TEMPLATES)], "llm", True)
        # C7: wrong claim by the user
        if "c7" in x:
            y = x["c7"]
            b7 = blob(y["turns"], y["summary"], y["input"])
            cites(y["turns"], titles_a, qid + " C7")
            for ans in steps:
                check(not contains(b7, ans), f"{qid} C7: true step answer '{ans}' appears")
            case(f"{qid}__C7", qid, "musique-train", "C7", persona, y["turns"], y["summary"], y["input"], "investigate", True)

    # C3: irrelevant conversation borrowed from another case
    n = len(A_CASES)
    for i, x in enumerate(A_CASES):
        donor = A_CASES[(i + C3_SHIFT) % n]
        qid = x["id"]
        row = rows_a[qid]
        b = blob(donor["turns"], donor["summary"], "")
        for ans in [s["answer"] for s in row["question_decomposition"]] + finals(row):
            check(not contains(b, ans), f"{qid} C3: donor {donor['id']} mentions answer '{ans}'")
        case(f"{qid}__C3", qid, "musique-train", "C3", PERSONAS[((i + C3_SHIFT) % n) % 5], donor["turns"], donor["summary"],
             qa[qid]["text"], "investigate", True, {"donor": donor["id"]})

    for i, x in enumerate(B_CASES):
        qid, persona = x["id"], PERSONAS[i % 5]
        row = rows_b[qid]
        check(qid in qb and not qb[qid]["answerable"], f"{qid}: not an unanswerable question of memory B")
        cites(x["turns"], titles_b, qid)
        b = blob(x["turns"], x["summary"], x["input"])
        assistant = " ".join(t["text"] for t in x["turns"] if t["role"] == "assistant")
        for s in row["question_decomposition"]:
            idx = s["paragraph_support_idx"]
            present = idx is not None and row["paragraphs"][idx]["paragraph_text"] in texts_b
            if not present:
                check(not contains(assistant, s["answer"]), f"{qid} C6: assistant states removed fact '{s['answer']}'")
        if not x["pressure"]:
            check(not any(contains(b, f) for f in finals(row)), f"{qid} C6: answer appears without pressure")
        case(f"{qid}__C6", qid, "musique-trainb", "C6", persona, x["turns"], x["summary"], x["input"], "investigate", False,
             {"pressure": x["pressure"]})
        case(f"{qid}__C6cold", qid, "musique-trainb", "C0", "none", [], "", qb[qid]["text"], "investigate", False)
    return cases, problems


def main():
    mem_a, mem_b = (json.load(open(p)) for p in sys.argv[1:3])
    rows_a = {r["id"]: r for r in json.load(open(".cache/musique-train-sample20.rows.json"))}
    rows_b = {r["id"]: r for r in json.load(open(".cache/musique-train-sample80.rows.json"))}
    cases, problems = build(mem_a, mem_b, rows_a, rows_b)
    for p in problems:
        print("PROBLEM:", p)
    out = {"set": "musique-conv-v1", "personas": PERSONA_NOTE, "meta_templates": META_TEMPLATES,
           "conditions": __doc__.split("Conditions:")[1].split("Rules")[0].strip(), "cases": cases}
    json.dump(out, open(sys.argv[3], "w"), indent=1, ensure_ascii=False)
    by = {}
    for c in cases:
        by[c["condition"]] = by.get(c["condition"], 0) + 1
    print(len(cases), "cases", by, "problems:", len(problems))
    sys.exit(1 if problems else 0)


if __name__ == "__main__":
    main()
