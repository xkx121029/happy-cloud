/// 数据模型：与 docs/api.md 契约一一对应
class User {
  final int id;
  final String username;
  final String email;
  final int role;
  final int quotaMax;
  final int quotaUsed;
  final int status;
  final String createdAt;

  const User({
    required this.id,
    required this.username,
    required this.email,
    required this.role,
    required this.quotaMax,
    required this.quotaUsed,
    required this.status,
    required this.createdAt,
  });

  factory User.fromJson(Map<String, dynamic> json) => User(
        id: (json['id'] as num?)?.toInt() ?? 0,
        username: json['username'] as String? ?? '',
        email: json['email'] as String? ?? '',
        role: (json['role'] as num?)?.toInt() ?? 0,
        quotaMax: (json['quota_max'] as num?)?.toInt() ?? 0,
        quotaUsed: (json['quota_used'] as num?)?.toInt() ?? 0,
        status: (json['status'] as num?)?.toInt() ?? 0,
        createdAt: json['created_at'] as String? ?? '',
      );

  bool get isAdmin => role == 1;
}

/// 文件/文件夹，type: 0 文件夹 / 1 文件
class FileItem {
  final int id;
  final int parentId;
  final String name;
  final int type;
  final int size;
  final String hash;
  final String createdAt;

  const FileItem({
    required this.id,
    required this.parentId,
    required this.name,
    required this.type,
    required this.size,
    required this.hash,
    required this.createdAt,
  });

  bool get isFolder => type == 0;

  factory FileItem.fromJson(Map<String, dynamic> json) => FileItem(
        id: (json['id'] as num?)?.toInt() ?? 0,
        parentId: (json['parent_id'] as num?)?.toInt() ?? 0,
        name: json['name'] as String? ?? '',
        type: (json['type'] as num?)?.toInt() ?? 1,
        size: (json['size'] as num?)?.toInt() ?? 0,
        hash: json['hash'] as String? ?? '',
        createdAt: json['created_at'] as String? ?? '',
      );
}

/// 分享信息
class ShareInfo {
  final String token;
  final String url;

  const ShareInfo({required this.token, required this.url});

  factory ShareInfo.fromJson(Map<String, dynamic> json) => ShareInfo(
        token: json['token'] as String? ?? '',
        url: json['url'] as String? ?? '',
      );
}

/// 分享预览
class ShareDetail {
  final FileItem? file;
  final List<FileItem> files;
  final bool passwordRequired;

  const ShareDetail({
    this.file,
    this.files = const [],
    required this.passwordRequired,
  });

  factory ShareDetail.fromJson(Map<String, dynamic> json) {
    final fileJson = json['file'];
    final filesJson = json['files'];
    return ShareDetail(
      file: fileJson is Map<String, dynamic> ? FileItem.fromJson(fileJson) : null,
      files: filesJson is List
          ? filesJson
              .whereType<Map<String, dynamic>>()
              .map(FileItem.fromJson)
              .toList()
          : const [],
      passwordRequired: json['password_required'] == true,
    );
  }
}
