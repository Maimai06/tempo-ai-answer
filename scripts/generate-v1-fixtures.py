"""仅维护期生成无凭据 v1 行为基准；发行程序与 Go CI 不依赖 Python。"""
import ast
import __future__
import html
import json
import re
from pathlib import Path
ns = {'re': re, 'html_lib': html}
for name in ['bot/solver.py', 'bot/ai_web.py']:
    tree = ast.parse(Path(name).read_text())
    nodes = [n for n in tree.body if isinstance(n, (ast.FunctionDef, ast.Assign))]
    exec(compile(ast.Module(body=nodes, type_ignores=[]), name, 'exec', flags=__future__.annotations.compiler_flag), ns)
step = {'courseStepId': 'fixture', 'title': '换行与中文', 'courseName': 'Java',
        'description': '说明 ![图](https://example.invalid/img){{width=2}} 完成代码',
        'questionType': 'code', 'questionInfo': {'questionCodeList': [
            {'codePath': 'src/Test.java', 'name': 'Test.java', 'code': 'class Test {\n/********* Begin *********/\n/********* End *********/\n}'}]},
        'trialSetList': [{'input': '', 'expectedOutput': '你好\n'}]}
fixtures = []
for multi in [False, True]:
    s = json.loads(json.dumps(step))
    if multi:
        s['questionInfo']['questionCodeList'].append({'codePath': 'A.java', 'name': 'A.java', 'code': 'class A {}'})
    for feedback in ['', '输出少了换行']:
        for seq in [0, 2]:
            fixtures.append({'kind': 'prompt', 'step': s, 'feedback': feedback, 'seq': seq,
                             'expected': ns['build_prompt'](s, feedback, seq)})
items = [{'questionId': 1, 'type': 'single_choice', 'title': '<p>1 &lt; 2？</p>', 'content': ['正确', '错误']}]
for feedback in ['', '第1题错误']:
    fixtures.append({'kind': 'select', 'items': items, 'feedback': feedback,
                     'expected': ns['build_select_prompt'](items, feedback)})
for text in ['', '<div>a<br>b</div><pre>  x &amp; y</pre>', '<p>甲</p>\n\n\n<span>乙</span>']:
    fixtures.append({'kind': 'html', 'text': text, 'expected': ns['html_to_text'](text)})
for text in ['1. b\n第2: CA\n3. F', 'A\nBC\nF', '', '1. A\n3. c', '99. A\n1: ba']:
    fixtures.append({'kind': 'choice', 'text': text, 'expected': ns['parse_choice_answers'](text, 3)})
for text in ['', '```java\nclass A {}\n```', '说明：代码\njava\nclass A {}\n以上即可', '```java\nx\n```\n```java\nclass Longer {}\n```']:
    fixtures.append({'kind': 'extract', 'text': text, 'expected': ns['extract_code'](text)})
for expected, actual in [('中文：\n', '中文:\n'), ('x\n', 'x'), ('x', 'x\n'), ('', ''), ('a b', 'a\tb'), ("it's", "it’s")]:
    fixtures.append({'kind': 'diff', 'expectedOutput': expected, 'actualOutput': actual,
                     'expected': ns['diff_hint'](expected, actual)})
Path('testdata/v1-parity.json').write_text(json.dumps(fixtures, ensure_ascii=False, indent=2)+'\n')
