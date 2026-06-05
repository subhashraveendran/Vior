#ifndef VIOR_TRAY_DARWIN_H
#define VIOR_TRAY_DARWIN_H

#ifdef __cplusplus
extern "C" {
#endif

void viorTrayInstall(void);
void viorTrayUninstall(void);
void viorTraySetStatus(const char *text, int running);

#ifdef __cplusplus
}
#endif

#endif
