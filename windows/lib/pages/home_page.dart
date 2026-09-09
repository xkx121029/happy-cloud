import 'dart:async';
import 'dart:io';

import 'package:desktop_drop/desktop_drop.dart';
import 'package:file_selector/file_selector.dart';
import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import '../core/api.dart';
import '../core/models.dart';
import '../state/app_state.dart';
import '../state/upload_service.dart';
import '../widgets/dialogs.dart';
import '../widgets/file_cell.dart';

/// 文件管理主页
class HomePage extends StatefulWidget {
  const HomePage({super.key});

  @override
  State<HomePage> createState() => _HomePageState();
}

class _HomePageState extends State<HomePage> {
  @override
  void initState() {
    super.initState();
    final state = context.read<AppState>();
    state.loadFiles();
    state.loadQuota();
  }

  // ---------- 上传 ----------

  Future<void> _pickAndUpload() async {
    final files = await openFiles();
    if (files.isEmpty) return;
    _startUploads(files.map((f) => f.path).toList());
  }

  void _startUploads(List<String> paths) {
    final state = context.read<AppState>();
    for (final p in paths) {
      final file = File(p);
      final task = UploadTask(
        path: p,
        name: p.split(Platform.pathSeparator).last,
        size: file.lengthSync(),
      );
      state.addUpload(task);
      unawaited(_runUpload(state, task));
    }
  }

  Future<void> _runUpload(AppState state, UploadTask task) async {
    try {
      await UploadService.instance.uploadFile(state, task);
    } catch (e) {
      task.status = UploadStatus.failed;
      task.error = e.toString();
      state.refresh();
    }
    await state.loadFiles();
    await state.loadQuota();
  }

  // ---------- 下载 ----------

  Future<void> _downloadFile(FileItem file) async {
    final location = await getSaveLocation(
      suggestedName: file.name,
      acceptedTypeGroups: const [],
    );
    if (location == null || location.path.isEmpty) return;
    if (!mounted) return;

    final progress = ValueNotifier<double>(0);
    final failed = ValueNotifier<bool>(false);
    showDialog<void>(
      context: context,
      barrierDismissible: false,
      builder: (ctx) => ValueListenableBuilder<bool>(
        valueListenable: failed,
        builder: (ctx, isFailed, _) => AlertDialog(
          title: const Text('下载'),
          content: SizedBox(
            width: 320,
            child: ValueListenableBuilder<double>(
              valueListenable: progress,
              builder: (ctx, p, _) => Column(
                mainAxisSize: MainAxisSize.min,
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(file.name, maxLines: 1, overflow: TextOverflow.ellipsis),
                  const SizedBox(height: 12),
                  LinearProgressIndicator(value: isFailed ? null : p),
                  const SizedBox(height: 8),
                  Text(
                    isFailed ? '下载失败' : '${(p * 100).toStringAsFixed(0)}%',
                    style: TextStyle(fontSize: 12, color: Colors.grey.shade600),
                  ),
                ],
              ),
            ),
          ),
          actions: [
            if (isFailed)
              TextButton(onPressed: () => Navigator.pop(ctx), child: const Text('关闭')),
          ],
        ),
      ),
    );

    try {
      await ApiClient.instance.download(
        '/files/download',
        location.path,
        query: {'file_id': file.id},
        onProgress: (received, total) {
          if (total > 0) progress.value = received / total;
        },
      );
      if (mounted) {
        Navigator.pop(context);
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text('已保存到：${location.path}')),
        );
      }
    } catch (e) {
      failed.value = true;
    }
  }

  // ---------- 文件操作分发 ----------

  Future<void> _onAction(FileItem file, FileAction action) async {
    final state = context.read<AppState>();
    switch (action) {
      case FileAction.download:
        if (!file.isFolder) {
          await _downloadFile(file);
          return;
        }
        return;
      case FileAction.rename:
        await showRenameDialog(context, state, file);
        return;
      case FileAction.move:
        final target = await showDialog<int>(
          context: context,
          builder: (_) => const FolderPickerDialog(),
        );
        if (target != null && target != file.parentId) {
          try {
            await state.move(file, target);
          } catch (e) {
            _toast('移动失败：$e');
          }
        }
        return;
      case FileAction.share:
        await showShareDialog(context, state, file);
        return;
      case FileAction.delete:
        if (await showDeleteConfirm(context, [file])) {
          try {
            await state.delete([file]);
          } catch (e) {
            _toast('删除失败：$e');
          }
        }
        return;
    }
  }

  void _toast(String msg) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(msg)));
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: const Color(0xFFF6F7F9),
      body: Stack(
        children: [
          DropTarget(
            onDragDone: (detail) {
              final paths = detail.files.map((f) => f.path).where((p) => p.isNotEmpty).toList();
              if (paths.isNotEmpty) _startUploads(paths);
            },
            onDragEntered: (_) => setState(() => _dragging = true),
            onDragExited: (_) => setState(() => _dragging = false),
            child: Column(
              children: [
                _Toolbar(onUpload: _pickAndUpload),
                Expanded(
                  child: Consumer<AppState>(
                    builder: (context, state, _) => _dragging
                        ? _buildDropOverlay()
                        : _buildFileArea(state),
                  ),
                ),
              ],
            ),
          ),
          // 上传任务面板
          Positioned(
            right: 16,
            bottom: 16,
            child: Consumer<AppState>(
              builder: (context, state, _) => state.uploads.isEmpty
                  ? const SizedBox.shrink()
                  : _UploadPanel(state: state),
            ),
          ),
        ],
      ),
    );
  }

  bool _dragging = false;

  Widget _buildDropOverlay() {
    return Container(
      margin: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: const Color(0x142563EB),
        borderRadius: BorderRadius.circular(16),
        border: Border.all(color: const Color(AppConfig.primary), width: 2),
      ),
      alignment: Alignment.center,
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          const Icon(Icons.file_upload_outlined, size: 48, color: Color(AppConfig.primary)),
          const SizedBox(height: 12),
          Text('松开即可上传到当前目录',
              style: TextStyle(fontSize: 15, color: Colors.grey.shade700)),
        ],
      ),
    );
  }

  Widget _buildFileArea(AppState state) {
    if (state.loading && state.files.isEmpty) {
      return const Center(child: CircularProgressIndicator());
    }
    if (state.error != null && state.files.isEmpty) {
      return Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(Icons.cloud_off, size: 40, color: Colors.grey.shade400),
            const SizedBox(height: 8),
            Text(state.error!, style: TextStyle(color: Colors.grey.shade600)),
            const SizedBox(height: 12),
            OutlinedButton(onPressed: state.loadFiles, child: const Text('重试')),
          ],
        ),
      );
    }
    if (state.files.isEmpty) {
      return Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(Icons.folder_open, size: 48, color: Colors.grey.shade400),
            const SizedBox(height: 12),
            Text('此目录为空', style: TextStyle(fontSize: 14, color: Colors.grey.shade500)),
            const SizedBox(height: 4),
            Text('点击上传或拖拽文件到窗口', style: TextStyle(fontSize: 12, color: Colors.grey.shade400)),
          ],
        ),
      );
    }

    if (state.gridMode) {
      return GridView.builder(
        padding: const EdgeInsets.all(16),
        gridDelegate: const SliverGridDelegateWithMaxCrossAxisExtent(
          maxCrossAxisExtent: 160,
          mainAxisSpacing: 12,
          crossAxisSpacing: 12,
          childAspectRatio: 0.95,
        ),
        itemCount: state.files.length,
        itemBuilder: (context, i) {
          final file = state.files[i];
          return FileGridCard(
            file: file,
            selected: file.id == _selectedId,
            onOpen: () => _open(state, file),
            onAction: _onAction,
          );
        },
      );
    }

    return Column(
      children: [
        const FileListHeader(),
        Expanded(
          child: ListView.builder(
            itemCount: state.files.length,
            itemBuilder: (context, i) {
              final file = state.files[i];
              return FileRow(
                file: file,
                selected: file.id == _selectedId,
                onTap: () => setState(() => _selectedId = file.id),
                onOpen: () => _open(state, file),
                onAction: _onAction,
              );
            },
          ),
        ),
      ],
    );
  }

  int? _selectedId;

  void _open(AppState state, FileItem file) {
    if (file.isFolder) {
      state.enterFolder(file);
    } else {
      _downloadFile(file);
    }
  }
}

/// 顶部工具条
class _Toolbar extends StatelessWidget {
  final VoidCallback onUpload;
  const _Toolbar({required this.onUpload});

  @override
  Widget build(BuildContext context) {
    return Container(
      height: 60,
      padding: const EdgeInsets.symmetric(horizontal: 16),
      decoration: BoxDecoration(
        color: Colors.white,
        border: Border(bottom: BorderSide(color: Colors.grey.shade200)),
      ),
      child: Consumer<AppState>(
        builder: (context, state, _) => Row(
          children: [
            // 面包屑导航
            _Breadcrumbs(state: state),
            const Spacer(),
            // 空间用量条
            _QuotaBar(state: state),
            const SizedBox(width: 16),
            // 视图切换
            IconButton(
              onPressed: () => context.read<AppState>().setGridMode(!state.gridMode),
              icon: Icon(state.gridMode ? Icons.view_list_outlined : Icons.grid_view_outlined),
              tooltip: state.gridMode ? '切换为列表视图' : '切换为网格视图',
            ),
            const SizedBox(width: 8),
            OutlinedButton.icon(
              onPressed: () => showMkdirDialog(context, context.read<AppState>()),
              icon: const Icon(Icons.create_new_folder_outlined, size: 18),
              label: const Text('新建文件夹'),
            ),
            const SizedBox(width: 8),
            FilledButton.icon(
              onPressed: onUpload,
              icon: const Icon(Icons.file_upload_outlined, size: 18),
              label: const Text('上传'),
            ),
            const SizedBox(width: 8),
            // 用户菜单
            PopupMenuButton<String>(
              tooltip: '用户菜单',
              offset: const Offset(0, 44),
              itemBuilder: (ctx) => [
                PopupMenuItem<String>(
                  enabled: false,
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(state.user?.username ?? '',
                          style: const TextStyle(fontWeight: FontWeight.w600)),
                      Text(state.user?.email ?? '',
                          style: TextStyle(fontSize: 12, color: Colors.grey.shade600)),
                    ],
                  ),
                ),
                const PopupMenuDivider(),
                PopupMenuItem<String>(
                  value: 'quota',
                  child: Row(
                    children: [
                      const Icon(Icons.storage_outlined, size: 18),
                      const SizedBox(width: 8),
                      Text('空间用量：${formatSize(state.quotaUsed)} / ${formatSize(state.quotaMax)}'),
                    ],
                  ),
                ),
                const PopupMenuDivider(),
                PopupMenuItem<String>(
                  value: 'logout',
                  child: const Row(
                    children: [
                      Icon(Icons.logout, size: 18),
                      SizedBox(width: 8),
                      Text('退出登录'),
                    ],
                  ),
                ),
              ],
              onSelected: (v) async {
                if (v == 'logout') {
                  await context.read<AppState>().logout();
                }
              },
              child: Padding(
                padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 6),
                child: Row(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    CircleAvatar(
                      radius: 15,
                      backgroundColor: const Color(0x1A2563EB),
                      child: Text(
                        (state.user?.username.isNotEmpty ?? false)
                            ? state.user!.username[0].toUpperCase()
                            : '?',
                        style: const TextStyle(
                          fontSize: 13,
                          fontWeight: FontWeight.w600,
                          color: Color(AppConfig.primary),
                        ),
                      ),
                    ),
                    const SizedBox(width: 8),
                    Text(
                      state.user?.username ?? '',
                      style: const TextStyle(fontSize: 14, fontWeight: FontWeight.w500),
                    ),
                    const Icon(Icons.arrow_drop_down),
                  ],
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

/// 面包屑
class _Breadcrumbs extends StatelessWidget {
  final AppState state;
  const _Breadcrumbs({required this.state});

  @override
  Widget build(BuildContext context) {
    return ConstrainedBox(
      constraints: const BoxConstraints(maxWidth: 520),
      child: SingleChildScrollView(
        scrollDirection: Axis.horizontal,
        child: Row(
          children: [
            for (var i = 0; i < state.crumbs.length; i++) ...[
              InkWell(
                onTap: i == state.crumbs.length - 1 ? null : () => state.jumpTo(i),
                borderRadius: BorderRadius.circular(6),
                child: Padding(
                  padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 6),
                  child: Text(
                    state.crumbs[i].name,
                    style: TextStyle(
                      fontSize: 14,
                      fontWeight: i == state.crumbs.length - 1 ? FontWeight.w600 : FontWeight.w400,
                      color: i == state.crumbs.length - 1
                          ? const Color(AppConfig.primary)
                          : Colors.grey.shade700,
                    ),
                  ),
                ),
              ),
              if (i < state.crumbs.length - 1)
                Icon(Icons.chevron_right, size: 18, color: Colors.grey.shade400),
            ],
          ],
        ),
      ),
    );
  }
}

/// 空间用量条
class _QuotaBar extends StatelessWidget {
  final AppState state;
  const _QuotaBar({required this.state});

  @override
  Widget build(BuildContext context) {
    final ratio = state.quotaMax > 0 ? (state.quotaUsed / state.quotaMax).clamp(0.0, 1.0) : 0.0;
    return SizedBox(
      width: 180,
      child: Column(
        mainAxisAlignment: MainAxisAlignment.center,
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            mainAxisAlignment: MainAxisAlignment.spaceBetween,
            children: [
              Text('空间用量',
                  style: TextStyle(fontSize: 11, color: Colors.grey.shade600)),
              Text(
                '${formatSize(state.quotaUsed)} / ${formatSize(state.quotaMax)}',
                style: TextStyle(fontSize: 11, color: Colors.grey.shade600),
              ),
            ],
          ),
          const SizedBox(height: 6),
          ClipRRect(
            borderRadius: BorderRadius.circular(4),
            child: LinearProgressIndicator(
              value: ratio,
              minHeight: 6,
              backgroundColor: const Color(0xFFE8EAF0),
              color: ratio > 0.9
                  ? Colors.red.shade500
                  : const Color(AppConfig.primary),
            ),
          ),
        ],
      ),
    );
  }
}

/// 上传任务面板
class _UploadPanel extends StatelessWidget {
  final AppState state;
  const _UploadPanel({required this.state});

  static String _statusText(UploadTask t) {
    switch (t.status) {
      case UploadStatus.waiting:
        return '等待中';
      case UploadStatus.hashing:
        return '计算校验值…';
      case UploadStatus.checking:
        return '秒传校验…';
      case UploadStatus.uploading:
        return '上传中 ${(t.progress * 100).toStringAsFixed(0)}%';
      case UploadStatus.merging:
        return '合并分片…';
      case UploadStatus.done:
        return '已完成';
      case UploadStatus.failed:
        return '失败：${t.error ?? ''}';
    }
  }

  @override
  Widget build(BuildContext context) {
    final active = state.uploads.where((t) => !t.isFinished).toList();
    final done = state.uploads.length - active.length;
    return Container(
      width: 320,
      constraints: const BoxConstraints(maxHeight: 320),
      decoration: BoxDecoration(
        color: Colors.white,
        borderRadius: BorderRadius.circular(12),
        boxShadow: [
          BoxShadow(
            color: Colors.black.withValues(alpha: 0.10),
            blurRadius: 20,
            offset: const Offset(0, 6),
          ),
        ],
      ),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Padding(
            padding: const EdgeInsets.fromLTRB(14, 10, 8, 6),
            child: Row(
              children: [
                Text('上传任务',
                    style: TextStyle(fontSize: 13, fontWeight: FontWeight.w600, color: Colors.grey.shade800)),
                const SizedBox(width: 8),
                if (done > 0)
                  Text('$done 个已完成',
                      style: TextStyle(fontSize: 12, color: Colors.grey.shade500)),
                const Spacer(),
                IconButton(
                  onPressed: state.clearUploads,
                  icon: const Icon(Icons.clear_all, size: 18),
                  tooltip: '清除已完成任务',
                  visualDensity: VisualDensity.compact,
                ),
              ],
            ),
          ),
          const Divider(height: 1),
          Flexible(
            child: ListView.builder(
              shrinkWrap: true,
              padding: const EdgeInsets.symmetric(vertical: 4),
              itemCount: state.uploads.length,
              itemBuilder: (context, i) {
                final t = state.uploads[i];
                return ListTile(
                  dense: true,
                  contentPadding: const EdgeInsets.symmetric(horizontal: 14),
                  title: Text(t.name, maxLines: 1, overflow: TextOverflow.ellipsis,
                      style: const TextStyle(fontSize: 13)),
                  subtitle: Padding(
                    padding: const EdgeInsets.only(top: 4),
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        LinearProgressIndicator(
                          value: t.status == UploadStatus.done || t.status == UploadStatus.failed
                              ? null
                              : t.progress,
                          minHeight: 4,
                          borderRadius: BorderRadius.circular(2),
                        ),
                        const SizedBox(height: 4),
                        Text(_statusText(t),
                            style: TextStyle(fontSize: 11, color: Colors.grey.shade500)),
                      ],
                    ),
                  ),
                  trailing: t.status == UploadStatus.done || t.status == UploadStatus.failed
                      ? IconButton(
                          onPressed: () => state.removeUpload(t),
                          icon: Icon(Icons.close, size: 16, color: Colors.grey.shade500),
                          visualDensity: VisualDensity.compact,
                        )
                      : null,
                );
              },
            ),
          ),
        ],
      ),
    );
  }
}
