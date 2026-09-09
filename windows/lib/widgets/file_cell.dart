import 'package:flutter/material.dart';

import '../core/api.dart';
import '../core/models.dart';

/// 文件类型图标与颜色
class FileIcon {
  static IconData of(FileItem f) {
    if (f.isFolder) return Icons.folder;
    final ext = f.name.contains('.') ? f.name.split('.').last.toLowerCase() : '';
    switch (ext) {
      case 'jpg':
      case 'jpeg':
      case 'png':
      case 'gif':
      case 'webp':
      case 'bmp':
      case 'svg':
        return Icons.image_outlined;
      case 'mp4':
      case 'mkv':
      case 'avi':
      case 'mov':
      case 'webm':
        return Icons.movie_outlined;
      case 'mp3':
      case 'wav':
      case 'flac':
      case 'aac':
        return Icons.music_note_outlined;
      case 'zip':
      case 'rar':
      case '7z':
      case 'tar':
      case 'gz':
        return Icons.folder_zip_outlined;
      case 'pdf':
        return Icons.picture_as_pdf_outlined;
      case 'doc':
      case 'docx':
        return Icons.description_outlined;
      case 'xls':
      case 'xlsx':
        return Icons.table_chart_outlined;
      case 'ppt':
      case 'pptx':
        return Icons.slideshow_outlined;
      case 'txt':
      case 'md':
        return Icons.notes;
      default:
        return Icons.insert_drive_file_outlined;
    }
  }

  static Color colorOf(FileItem f) {
    if (f.isFolder) return const Color(0xFFF59E0B);
    final ext = f.name.contains('.') ? f.name.split('.').last.toLowerCase() : '';
    switch (ext) {
      case 'jpg':
      case 'jpeg':
      case 'png':
      case 'gif':
      case 'webp':
      case 'bmp':
      case 'svg':
        return const Color(0xFF8B5CF6);
      case 'mp4':
      case 'mkv':
      case 'avi':
      case 'mov':
        return const Color(0xFFEC4899);
      case 'mp3':
      case 'wav':
        return const Color(0xFF10B981);
      case 'zip':
      case 'rar':
      case '7z':
        return const Color(0xFFF97316);
      case 'pdf':
        return const Color(0xFFEF4444);
      default:
        return const Color(0xFF64748B);
    }
  }
}

/// 文件操作
enum FileAction { download, rename, move, share, delete }

/// 操作回调
typedef FileActions = void Function(FileItem file, FileAction action);

/// 文件列表行（表格视图）
class FileRow extends StatelessWidget {
  final FileItem file;
  final bool selected;
  final VoidCallback onOpen;
  final FileActions onAction;
  final VoidCallback? onTap;

  const FileRow({
    super.key,
    required this.file,
    required this.selected,
    required this.onOpen,
    required this.onAction,
    this.onTap,
  });

  void _showContextMenu(BuildContext context) {
    showMenu<FileAction>(
      context: context,
      position: RelativeRect.fromLTRB(0, 0, 0, 0),
      items: _menuItems(context, file),
    ).then((a) {
      if (a != null) onAction(file, a);
    });
  }

  static List<PopupMenuEntry<FileAction>> _menuItems(BuildContext context, FileItem file) {
    return [
      if (!file.isFolder)
        const PopupMenuItem(value: FileAction.download, child: Text('下载')),
      const PopupMenuItem(value: FileAction.rename, child: Text('重命名')),
      const PopupMenuItem(value: FileAction.move, child: Text('移动')),
      const PopupMenuItem(value: FileAction.share, child: Text('分享')),
      const PopupMenuItem(value: FileAction.delete, child: Text('删除')),
    ];
  }

  @override
  Widget build(BuildContext context) {
    return InkWell(
      onTap: onTap ?? onOpen,
      onDoubleTap: onOpen,
      onSecondaryTapDown: (_) => _showContextMenu(context),
      child: Container(
        height: 52,
        padding: const EdgeInsets.symmetric(horizontal: 16),
        decoration: BoxDecoration(
          color: selected ? const Color(0x142563EB) : Colors.white,
          border: Border(
            bottom: BorderSide(color: Colors.grey.shade200, width: 1),
          ),
        ),
        child: Row(
          children: [
            Icon(FileIcon.of(file), size: 22, color: FileIcon.colorOf(file)),
            const SizedBox(width: 12),
            Expanded(
              child: Text(
                file.name,
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                style: TextStyle(
                  fontSize: 14,
                  fontWeight: selected ? FontWeight.w600 : FontWeight.w400,
                  color: Colors.grey.shade900,
                ),
              ),
            ),
            SizedBox(
              width: 110,
              child: Text(
                file.isFolder ? '—' : formatSize(file.size),
                textAlign: TextAlign.right,
                style: TextStyle(fontSize: 13, color: Colors.grey.shade600),
              ),
            ),
            SizedBox(
              width: 150,
              child: Text(
                file.createdAt.isEmpty
                    ? ''
                    : file.createdAt.replaceFirst('T', ' ').substring(0, 16),
                textAlign: TextAlign.right,
                style: TextStyle(fontSize: 13, color: Colors.grey.shade500),
              ),
            ),
            const SizedBox(width: 8),
            _ActionButtons(file: file, onAction: onAction),
            const SizedBox(width: 4),
            IconButton(
              onPressed: () => _showContextMenu(context),
              icon: const Icon(Icons.more_vert, size: 18),
              tooltip: '更多操作',
              visualDensity: VisualDensity.compact,
            ),
          ],
        ),
      ),
    );
  }
}

class _ActionButtons extends StatelessWidget {
  final FileItem file;
  final FileActions onAction;
  const _ActionButtons({required this.file, required this.onAction});

  @override
  Widget build(BuildContext context) {
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        if (!file.isFolder)
          IconButton(
            onPressed: () => onAction(file, FileAction.download),
            icon: const Icon(Icons.download_outlined, size: 18),
            tooltip: '下载',
            visualDensity: VisualDensity.compact,
          ),
        IconButton(
          onPressed: () => onAction(file, FileAction.rename),
          icon: const Icon(Icons.drive_file_rename_outline, size: 18),
          tooltip: '重命名',
          visualDensity: VisualDensity.compact,
        ),
        IconButton(
          onPressed: () => onAction(file, FileAction.move),
          icon: const Icon(Icons.drive_file_move_outline, size: 18),
          tooltip: '移动',
          visualDensity: VisualDensity.compact,
        ),
        IconButton(
          onPressed: () => onAction(file, FileAction.share),
          icon: const Icon(Icons.share_outlined, size: 18),
          tooltip: '分享',
          visualDensity: VisualDensity.compact,
        ),
        IconButton(
          onPressed: () => onAction(file, FileAction.delete),
          icon: const Icon(Icons.delete_outline, size: 18),
          tooltip: '删除',
          visualDensity: VisualDensity.compact,
        ),
      ],
    );
  }
}

/// 文件网格卡片
class FileGridCard extends StatelessWidget {
  final FileItem file;
  final bool selected;
  final VoidCallback onOpen;
  final FileActions onAction;

  const FileGridCard({
    super.key,
    required this.file,
    required this.selected,
    required this.onOpen,
    required this.onAction,
  });

  void _showContextMenu(BuildContext context) {
    showMenu<FileAction>(
      context: context,
      position: RelativeRect.fromLTRB(0, 0, 0, 0),
      items: [
        if (!file.isFolder)
          const PopupMenuItem(value: FileAction.download, child: Text('下载')),
        const PopupMenuItem(value: FileAction.rename, child: Text('重命名')),
        const PopupMenuItem(value: FileAction.move, child: Text('移动')),
        const PopupMenuItem(value: FileAction.share, child: Text('分享')),
        const PopupMenuItem(value: FileAction.delete, child: Text('删除')),
      ],
    ).then((a) {
      if (a != null) onAction(file, a);
    });
  }

  @override
  Widget build(BuildContext context) {
    return InkWell(
      onTap: () => _showContextMenu(context),
      onDoubleTap: onOpen,
      onSecondaryTapDown: (_) => _showContextMenu(context),
      borderRadius: BorderRadius.circular(12),
      child: Container(
        decoration: BoxDecoration(
          color: selected ? const Color(0x142563EB) : Colors.white,
          borderRadius: BorderRadius.circular(12),
          border: Border.all(
            color: selected ? const Color(AppConfig.primary) : Colors.grey.shade200,
          ),
        ),
        padding: const EdgeInsets.all(14),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Icon(FileIcon.of(file), size: 34, color: FileIcon.colorOf(file)),
            const Spacer(),
            Text(
              file.name,
              maxLines: 2,
              overflow: TextOverflow.ellipsis,
              style: TextStyle(
                fontSize: 13,
                fontWeight: FontWeight.w500,
                color: Colors.grey.shade900,
              ),
            ),
            const SizedBox(height: 4),
            Text(
              file.isFolder ? '文件夹' : formatSize(file.size),
              style: TextStyle(fontSize: 12, color: Colors.grey.shade500),
            ),
          ],
        ),
      ),
    );
  }
}

/// 表格视图表头
class FileListHeader extends StatelessWidget {
  const FileListHeader({super.key});

  @override
  Widget build(BuildContext context) {
    return Container(
      height: 40,
      padding: const EdgeInsets.symmetric(horizontal: 16),
      decoration: BoxDecoration(
        color: const Color(0xFFF9FAFB),
        border: Border(bottom: BorderSide(color: Colors.grey.shade200)),
      ),
      child: const Row(
        children: [
          SizedBox(width: 34),
          Expanded(
            child: Text(
              '名称',
              style: TextStyle(
                fontSize: 13,
                fontWeight: FontWeight.w600,
                color: Colors.black54,
              ),
            ),
          ),
          SizedBox(
            width: 110,
            child: Text(
              '大小',
              textAlign: TextAlign.right,
              style: TextStyle(fontSize: 13, fontWeight: FontWeight.w600, color: Colors.black54),
            ),
          ),
          SizedBox(
            width: 150,
            child: Text(
              '修改时间',
              textAlign: TextAlign.right,
              style: TextStyle(fontSize: 13, fontWeight: FontWeight.w600, color: Colors.black54),
            ),
          ),
          SizedBox(width: 220),
          SizedBox(width: 40),
        ],
      ),
    );
  }
}
