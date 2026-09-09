import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../core/api.dart';
import '../core/models.dart';
import '../state/app_state.dart';

/// 通用输入对话框
Future<String?> showTextInputDialog(
  BuildContext context, {
  required String title,
  String? initialValue,
  String hint = '',
  String confirmText = '确定',
}) {
  final controller = TextEditingController(text: initialValue ?? '');
  return showDialog<String>(
    context: context,
    builder: (ctx) => AlertDialog(
      title: Text(title),
      content: TextField(
        controller: controller,
        autofocus: true,
        decoration: InputDecoration(hintText: hint),
        onSubmitted: (v) => Navigator.pop(ctx, v.trim()),
      ),
      actions: [
        TextButton(onPressed: () => Navigator.pop(ctx), child: const Text('取消')),
        FilledButton(
          onPressed: () => Navigator.pop(ctx, controller.text.trim()),
          child: Text(confirmText),
        ),
      ],
    ),
  );
}

/// 新建文件夹对话框
Future<void> showMkdirDialog(BuildContext context, AppState state) async {
  final name = await showTextInputDialog(
    context,
    title: '新建文件夹',
    hint: '请输入文件夹名称',
  );
  if (name == null || name.isEmpty) return;
  try {
    await state.mkdir(name);
  } catch (e) {
    if (context.mounted) {
      ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('创建失败：$e')));
    }
  }
}

/// 重命名对话框
Future<void> showRenameDialog(BuildContext context, AppState state, FileItem file) async {
  final name = await showTextInputDialog(
    context,
    title: '重命名',
    initialValue: file.name,
    confirmText: '重命名',
  );
  if (name == null || name.isEmpty || name == file.name) return;
  try {
    await state.rename(file, name);
  } catch (e) {
    if (context.mounted) {
      ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('重命名失败：$e')));
    }
  }
}

/// 删除确认对话框
Future<bool> showDeleteConfirm(BuildContext context, List<FileItem> items) async {
  if (items.isEmpty) return false;
  final name = items.length == 1 ? items.first.name : '${items.length} 个项目';
  final result = await showDialog<bool>(
    context: context,
    builder: (ctx) => AlertDialog(
      title: const Text('删除确认'),
      content: Text('确定要删除“$name”吗？删除后不可恢复。'),
      actions: [
        TextButton(onPressed: () => Navigator.pop(ctx, false), child: const Text('取消')),
        FilledButton(
          style: FilledButton.styleFrom(backgroundColor: Colors.red.shade600),
          onPressed: () => Navigator.pop(ctx, true),
          child: const Text('删除'),
        ),
      ],
    ),
  );
  return result ?? false;
}

/// 目录选择器（移动用）：逐级浏览文件夹
class FolderPickerDialog extends StatefulWidget {
  final int initialId;
  const FolderPickerDialog({super.key, this.initialId = 0});

  @override
  State<FolderPickerDialog> createState() => _FolderPickerDialogState();
}

class _FolderPickerDialogState extends State<FolderPickerDialog> {
  final List<Crumb> _crumbs = [const Crumb(0, '全部文件')];
  List<FileItem> _folders = [];
  bool _loading = false;

  int get _currentId => _crumbs.last.id;

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    setState(() => _loading = true);
    try {
      final data = await ApiClient.instance.get('/files/list', query: {
        'parent_id': _currentId,
        'page': 1,
        'page_size': 100,
      });
      final list = data['list'] ?? data['files'] ?? data['items'] ?? [];
      final all = list
          .whereType<Map<String, dynamic>>()
          .map(FileItem.fromJson)
          .toList();
      setState(() => _folders = all.where((f) => f.isFolder).toList());
    } catch (e) {
      setState(() => _folders = []);
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  void _enter(FileItem folder) {
    setState(() => _crumbs.add(Crumb(folder.id, folder.name)));
    _load();
  }

  void _back() {
    if (_crumbs.length > 1) {
      setState(() => _crumbs.removeLast());
      _load();
    }
  }

  @override
  Widget build(BuildContext context) {
    return AlertDialog(
      title: const Text('移动到'),
      content: SizedBox(
        width: 360,
        height: 360,
        child: Column(
          children: [
            Row(
              children: [
                IconButton(
                  onPressed: _crumbs.length > 1 ? _back : null,
                  icon: const Icon(Icons.arrow_upward),
                  tooltip: '上级目录',
                ),
                Expanded(
                  child: SingleChildScrollView(
                    scrollDirection: Axis.horizontal,
                    child: Wrap(
                      crossAxisAlignment: WrapCrossAlignment.center,
                      children: [
                        for (var i = 0; i < _crumbs.length; i++) ...[
                          InkWell(
                            onTap: () {
                              if (i < _crumbs.length - 1) {
                                setState(() => _crumbs.removeRange(i + 1, _crumbs.length));
                                _load();
                              }
                            },
                            child: Padding(
                              padding: const EdgeInsets.symmetric(horizontal: 2, vertical: 6),
                              child: Text(
                                _crumbs[i].name,
                                style: TextStyle(
                                  fontWeight: i == _crumbs.length - 1
                                      ? FontWeight.w600
                                      : FontWeight.normal,
                                  color: i == _crumbs.length - 1
                                      ? const Color(AppConfig.primary)
                                      : Colors.grey.shade700,
                                ),
                              ),
                            ),
                          ),
                          if (i < _crumbs.length - 1)
                            Icon(Icons.chevron_right, size: 16, color: Colors.grey.shade400),
                        ],
                      ],
                    ),
                  ),
                ),
              ],
            ),
            const Divider(height: 1),
            Expanded(
              child: _loading
                  ? const Center(child: CircularProgressIndicator())
                  : _folders.isEmpty
                      ? Center(child: Text('此目录下没有子文件夹', style: TextStyle(color: Colors.grey.shade500)))
                      : ListView.builder(
                          itemCount: _folders.length,
                          itemBuilder: (ctx, i) {
                            final folder = _folders[i];
                            return ListTile(
                              dense: true,
                              leading: Icon(Icons.folder, color: Colors.amber.shade700),
                              title: Text(folder.name),
                              onTap: () => _enter(folder),
                            );
                          },
                        ),
            ),
          ],
        ),
      ),
      actions: [
        TextButton(onPressed: () => Navigator.pop(context, null), child: const Text('取消')),
        FilledButton(
          onPressed: () => Navigator.pop(context, _currentId),
          child: const Text('移动到此处'),
        ),
      ],
    );
  }
}

/// 分享对话框：创建链接 + 复制 + 预览
Future<void> showShareDialog(BuildContext context, AppState state, FileItem file) async {
  final passwordCtrl = TextEditingController();
  int? expireDays;

  final created = await showDialog<ShareInfo>(
    context: context,
    builder: (ctx) => StatefulBuilder(
      builder: (ctx, setState) => AlertDialog(
        title: Text('分享「${file.name}」'),
        content: SizedBox(
          width: 380,
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              TextField(
                controller: passwordCtrl,
                obscureText: true,
                decoration: const InputDecoration(
                  labelText: '访问密码（可选）',
                  hintText: '留空表示无需密码',
                ),
              ),
              const SizedBox(height: 12),
              DropdownButtonFormField<int?>(
                initialValue: expireDays,
                decoration: const InputDecoration(labelText: '有效期'),
                items: const [
                  DropdownMenuItem<int?>(value: null, child: Text('永久有效')),
                  DropdownMenuItem<int?>(value: 1, child: Text('1 天')),
                  DropdownMenuItem<int?>(value: 7, child: Text('7 天')),
                  DropdownMenuItem<int?>(value: 30, child: Text('30 天')),
                ],
                onChanged: (v) => setState(() => expireDays = v),
              ),
            ],
          ),
        ),
        actions: [
          TextButton(onPressed: () => Navigator.pop(ctx), child: const Text('取消')),
          FilledButton(
            onPressed: () async {
              try {
                final info = await state.createShare(
                  file,
                  password: passwordCtrl.text,
                  expireAt: expireDays == null
                      ? null
                      : DateTime.now()
                          .add(Duration(days: expireDays!))
                          .toIso8601String(),
                );
                if (ctx.mounted) Navigator.pop(ctx, info);
              } catch (e) {
                if (ctx.mounted) {
                  ScaffoldMessenger.of(ctx)
                      .showSnackBar(SnackBar(content: Text('创建分享失败：$e')));
                }
              }
            },
            child: const Text('创建链接'),
          ),
        ],
      ),
    ),
  );

  if (created == null || !context.mounted) return;

  await showDialog<void>(
    context: context,
    builder: (ctx) => AlertDialog(
      title: const Text('分享链接已创建'),
      content: SizedBox(
        width: 420,
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text('链接：', style: TextStyle(color: Colors.grey.shade600, fontSize: 13)),
            const SizedBox(height: 4),
            SelectableText(created.url, style: const TextStyle(color: Color(AppConfig.primary))),
            const SizedBox(height: 16),
            Row(
              children: [
                Expanded(
                  child: OutlinedButton.icon(
                    onPressed: () {
                      Clipboard.setData(ClipboardData(text: created.url));
                      ScaffoldMessenger.of(ctx)
                          .showSnackBar(const SnackBar(content: Text('链接已复制到剪贴板')));
                    },
                    icon: const Icon(Icons.copy, size: 16),
                    label: const Text('复制链接'),
                  ),
                ),
                const SizedBox(width: 8),
                Expanded(
                  child: FilledButton.icon(
                    onPressed: () {
                      Navigator.pop(ctx);
                      showSharePreviewDialog(ctx, created.token);
                    },
                    icon: const Icon(Icons.visibility_outlined, size: 16),
                    label: const Text('预览分享'),
                  ),
                ),
              ],
            ),
          ],
        ),
      ),
      actions: [
        TextButton(onPressed: () => Navigator.pop(ctx), child: const Text('关闭')),
      ],
    ),
  );
}

/// 分享链接预览（支持密码校验）
class SharePreviewDialog extends StatefulWidget {
  final String token;
  const SharePreviewDialog({super.key, required this.token});

  @override
  State<SharePreviewDialog> createState() => _SharePreviewDialogState();
}

class _SharePreviewDialogState extends State<SharePreviewDialog> {
  ShareDetail? _detail;
  String? _error;
  bool _loaded = false;
  final _passwordCtrl = TextEditingController();

  @override
  void initState() {
    super.initState();
    _load(null);
  }

  Future<void> _load(String? password) async {
    setState(() {
      _loaded = false;
      _error = null;
    });
    try {
      if (password != null) {
        await ApiClient.instance
            .post('/share/${widget.token}/verify', {'password': password});
      }
      final data = await ApiClient.instance.get('/share/${widget.token}');
      if (!mounted) return;
      setState(() => _detail = ShareDetail.fromJson(data));
    } catch (e) {
      if (!mounted) return;
      setState(() => _error = e.toString());
    } finally {
      if (mounted) setState(() => _loaded = true);
    }
  }

  @override
  Widget build(BuildContext context) {
    Widget content;
    if (!_loaded) {
      content = const Padding(
        padding: EdgeInsets.all(32),
        child: Center(child: CircularProgressIndicator()),
      );
    } else if (_detail == null) {
      content = Padding(
        padding: const EdgeInsets.all(8),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text('访问分享内容需要密码', style: TextStyle(color: Colors.grey.shade700)),
            const SizedBox(height: 12),
            TextField(
              controller: _passwordCtrl,
              obscureText: true,
              onSubmitted: (_) => _load(_passwordCtrl.text),
              decoration: const InputDecoration(labelText: '密码'),
            ),
            if (_error != null) ...[
              const SizedBox(height: 8),
              Text('提示：$_error',
                  style: TextStyle(color: Colors.red.shade600, fontSize: 12)),
            ],
          ],
        ),
      );
    } else {
      final d = _detail!;
      final entries = <Widget>[];
      if (d.file != null) {
        entries.add(_shareRow('名称', d.file!.name));
        entries.add(_shareRow('类型', d.file!.isFolder ? '文件夹' : '文件'));
        if (!d.file!.isFolder) {
          entries.add(_shareRow('大小', formatSize(d.file!.size)));
        }
      }
      for (final f in d.files) {
        entries.add(_shareRow('文件', '${f.name}（${formatSize(f.size)}）'));
      }
      content = Column(mainAxisSize: MainAxisSize.min, children: entries);
    }

    return AlertDialog(
      title: const Text('分享预览'),
      content: SizedBox(width: 380, child: content),
      actions: [
        TextButton(onPressed: () => Navigator.pop(context), child: const Text('关闭')),
        if (_detail == null && _error != null)
          FilledButton(
            onPressed: () => _load(_passwordCtrl.text),
            child: const Text('验证密码'),
          ),
      ],
    );
  }
}

Future<void> showSharePreviewDialog(BuildContext context, String token) {
  return showDialog<void>(
    context: context,
    builder: (_) => SharePreviewDialog(token: token),
  );
}

Widget _shareRow(String label, String value) {
  return Padding(
    padding: const EdgeInsets.symmetric(vertical: 6),
    child: Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        SizedBox(
          width: 90,
          child: Text(label, style: const TextStyle(color: Colors.black54)),
        ),
        Expanded(child: Text(value)),
      ],
    ),
  );
}
